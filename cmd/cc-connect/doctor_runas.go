package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chenhg5/cc-connect/config"
	"github.com/chenhg5/cc-connect/core"
)

// runDoctor dispatches `cc-connect doctor ...`. Today the only subcommand
// is `user-isolation`, but this function is the growth point for future
// diagnostics.
func runDoctor(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cc-connect doctor <subcommand>")
		fmt.Fprintln(os.Stderr, "subcommands:")
		fmt.Fprintln(os.Stderr, "  user-isolation   audit run_as_user projects and emit an isolation report")
		os.Exit(2)
	}
	switch args[0] {
	case "user-isolation":
		runDoctorUserIsolation(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown doctor subcommand %q\n", args[0])
		os.Exit(2)
	}
}

// runDoctorUserIsolation runs preflight + isolation probe for one or all
// projects that have run_as_user set, writes a JSON report per project,
// and exits 0 on full clean, 1 otherwise.
func runDoctorUserIsolation(args []string) {
	fs := flag.NewFlagSet("doctor user-isolation", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file (default: auto-discover)")
	projectFilter := fs.String("project", "", "limit audit to a single project name")
	outPath := fs.String("out", "", "path to write JSON report (default: ~/.cc-connect/audits/<timestamp>-<project>.json per project)")
	printScript := fs.Bool("print-script", false, "print the embedded probe script and exit")
	_ = fs.Parse(args)

	if *printScript {
		os.Stdout.Write(core.ProbeScript())
		return
	}

	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "doctor user-isolation: run_as_user is not supported on Windows")
		os.Exit(1)
	}

	cfgPath := resolveConfigPath(*configPath)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config %s: %v\n", cfgPath, err)
		os.Exit(1)
	}

	// Collect projects with run_as_user set (optionally filtered).
	type pending struct {
		project   string
		runAsUser string
		workDir   string
	}
	var targets []pending
	var allUsers []string
	for _, proj := range cfg.Projects {
		if proj.RunAsUser == "" {
			continue
		}
		allUsers = append(allUsers, proj.RunAsUser)
	}
	for _, proj := range cfg.Projects {
		if proj.RunAsUser == "" {
			continue
		}
		if *projectFilter != "" && proj.Name != *projectFilter {
			continue
		}
		wd, _ := proj.Agent.Options["work_dir"].(string)
		targets = append(targets, pending{
			project:   proj.Name,
			runAsUser: proj.RunAsUser,
			workDir:   wd,
		})
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "doctor user-isolation: no projects with run_as_user set")
		if *projectFilter != "" {
			fmt.Fprintf(os.Stderr, "  (filter: --project %q)\n", *projectFilter)
		}
		os.Exit(0)
	}

	supervisor := ""
	if u, err := user.Current(); err == nil {
		supervisor = u.Username
	}

	runner := core.ExecSudoRunner{}
	exitCode := 0
	for _, t := range targets {
		fmt.Printf("=== %s (run_as_user = %s) ===\n", t.project, t.runAsUser)

		// Preflight.
		pfCtx, pfCancel := context.WithTimeout(context.Background(), 30*time.Second)
		pf := core.PreflightRunAsUser(pfCtx, core.PreflightConfig{
			Project:   t.project,
			RunAsUser: t.runAsUser,
			WorkDir:   t.workDir,
			Runner:    runner,
		})
		pfCancel()

		for _, w := range pf.Warnings {
			fmt.Printf("[WARN] %s\n", w)
		}
		for _, f := range pf.Fatal {
			fmt.Printf("[FATAL] %s\n", f)
		}
		if pf.HasFatal() {
			exitCode = 1
			fmt.Println()
			continue
		}
		fmt.Println("preflight: OK")

		// Audit probe.
		audCtx, audCancel := context.WithTimeout(context.Background(), 20*time.Second)
		report, err := core.RunIsolationProbe(audCtx, core.AuditConfig{
			Project:    t.project,
			RunAsUser:  t.runAsUser,
			WorkDir:    t.workDir,
			OtherUsers: allUsers,
			Supervisor: supervisor,
			Runner:     runner,
		})
		audCancel()
		if err != nil {
			fmt.Printf("[FATAL] probe failed to run: %v\n", err)
			exitCode = 1
			fmt.Println()
			continue
		}

		printHumanReport(report)

		// Write JSON report.
		dest := *outPath
		if dest == "" {
			dir, derr := defaultAuditDir()
			if derr != nil {
				fmt.Fprintf(os.Stderr, "could not determine audit output dir: %v\n", derr)
				exitCode = 1
				continue
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "could not create audit dir %s: %v\n", dir, err)
				exitCode = 1
				continue
			}
			ts := report.Timestamp.Format("20060102-150405")
			dest = filepath.Join(dir, fmt.Sprintf("%s-%s.json", ts, t.project))
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON marshal failed: %v\n", err)
			exitCode = 1
			continue
		}
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "writing %s: %v\n", dest, err)
			exitCode = 1
			continue
		}
		fmt.Printf("report written: %s\n", dest)

		if report.HasFatal() {
			exitCode = 1
		}
		fmt.Println()
	}

	os.Exit(exitCode)
}

// printHumanReport dumps a compact human-friendly summary of an audit to
// stdout. The JSON file is the authoritative record; this is for eyeballs.
func printHumanReport(r core.IsolationReport) {
	fmt.Printf("whoami         : %s\n", r.Identity.Whoami)
	fmt.Printf("id             : %s\n", r.Identity.ID)
	fmt.Printf("home           : %s\n", r.Identity.Home)
	fmt.Printf("workdir        : %s (readable=%v writable=%v)\n",
		r.WorkDirStatus.Path, r.WorkDirStatus.Readable, r.WorkDirStatus.Writable)

	hasCount, missCount := 0, 0
	for _, p := range r.TargetPaths {
		if p.Status == "has" {
			hasCount++
		} else {
			missCount++
		}
	}
	fmt.Printf("target home    : %d present, %d missing\n", hasCount, missCount)
	if missCount > 0 {
		for _, p := range r.TargetPaths {
			if p.Status == "missing" {
				fmt.Printf("  missing: %s\n", p.Path)
			}
		}
	}

	denied, leaked := 0, 0
	for _, c := range r.CrossUser {
		switch c.Status {
		case "denied":
			denied++
		case "leaked":
			leaked++
		}
	}
	fmt.Printf("cross-user     : %d denied, %d leaked\n", denied, leaked)
	for _, c := range r.CrossUser {
		if c.Status == "leaked" {
			fmt.Printf("  LEAKED: %s can read %s (%s)\n", r.RunAsUser, c.Path, c.OtherUser)
		}
	}

	supDenied, supLeaked := 0, 0
	for _, s := range r.Supervisor {
		switch s.Status {
		case "denied":
			supDenied++
		case "leaked":
			supLeaked++
		}
	}
	fmt.Printf("supervisor     : %d denied, %d leaked\n", supDenied, supLeaked)
	for _, s := range r.Supervisor {
		if s.Status == "leaked" {
			fmt.Printf("  LEAKED: %s can read supervisor's %s\n", r.RunAsUser, s.Path)
		}
	}

	if r.HasFatal() {
		fmt.Println("audit          : FATAL")
		for _, f := range r.Fatal {
			fmt.Printf("  %s\n", f)
		}
	} else {
		fmt.Println("audit          : OK")
	}
}

// defaultAuditDir returns ~/.cc-connect/audits for the supervisor user.
func defaultAuditDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cc-connect", "audits"), nil
}
