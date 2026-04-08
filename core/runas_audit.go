//go:build !windows

package core

// runas_audit.go — isolation leak-audit probe for the run_as_user sandbox.
//
// The preflight gates in runas_check.go answer the question "can
// cc-connect spawn as the target user without errors?". This file
// answers the stronger question: "once the target user IS spawned, can
// it still read things it shouldn't be able to?".
//
// We do that by running a fixed shell script inside the target user's
// sudo -i session and parsing its output into a structured report. The
// script (runas_probe.sh) is embedded via //go:embed so it ships with the
// binary and can be audited with shellcheck.
//
// # Failure policy
//
// Per the spec: unexpected audit outcomes are FATAL. Specifically:
//
//   - Any CROSS_LEAKED (the target user can read another project user's
//     secrets) is fatal.
//   - Any SUPERVISOR_LEAKED (the target user can read the supervisor's
//     secrets) is fatal.
//   - WORKDIR_WRITABLE=no is fatal (already caught by preflight, but
//     we assert it here too as defense in depth).
//
// Everything else is informational and stored in the report but does
// not block startup.

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

//go:embed runas_probe.sh
var runasProbeScript []byte

// ProbeScript returns the embedded probe script contents, primarily so
// the doctor subcommand can print it for user inspection.
func ProbeScript() []byte { return runasProbeScript }

// IsolationReport is the structured result of running the probe inside
// the target user's sudo session.
type IsolationReport struct {
	Project   string    `json:"project"`
	RunAsUser string    `json:"run_as_user"`
	WorkDir   string    `json:"work_dir"`
	Timestamp time.Time `json:"timestamp"`

	// Identity captures id/groups/umask/pwd as reported by the probe.
	Identity IdentitySnapshot `json:"identity"`

	// WorkDirStatus reports existence/readability/writability of the
	// project's work_dir as observed from the target user's context.
	WorkDirStatus WorkDirStatus `json:"work_dir_status"`

	// TargetPaths lists per-path existence results for files the target
	// user is SUPPOSED to have in their own home (~/.claude/settings.json,
	// ~/keys/, etc.). Missing is informational — tools will fail at
	// runtime but that's on the operator's migration, not a security hole.
	TargetPaths []PathStatus `json:"target_paths"`

	// CrossUser holds denial/leak results for cross-project reads.
	CrossUser []CrossUserResult `json:"cross_user"`

	// Supervisor holds denial/leak results for supervisor-user reads.
	Supervisor []PathStatus `json:"supervisor"`

	// Fatal lists audit-level fatal problems (any CROSS_LEAKED, any
	// SUPERVISOR_LEAKED, or a writability regression).
	Fatal []string `json:"fatal,omitempty"`

	// ProbeVersion is the version string reported by the probe's BEGIN
	// line, used for schema migrations.
	ProbeVersion string `json:"probe_version"`

	// RawOutput is the verbatim probe stdout. Useful for manual audit
	// and for debugging parser gaps.
	RawOutput string `json:"raw_output,omitempty"`
}

// HasFatal reports whether the audit surfaced any fatal problems.
func (r IsolationReport) HasFatal() bool { return len(r.Fatal) > 0 }

// IdentitySnapshot mirrors the probe's identity fields.
type IdentitySnapshot struct {
	ID     string `json:"id"`
	Whoami string `json:"whoami"`
	Groups string `json:"groups"`
	Umask  string `json:"umask"`
	Pwd    string `json:"pwd"`
	Home   string `json:"home"`
	Shell  string `json:"shell"`
}

// WorkDirStatus mirrors the probe's WORKDIR_* fields.
type WorkDirStatus struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
}

// PathStatus is a single target_paths or supervisor path result.
type PathStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"` // has | missing | denied | leaked
}

// CrossUserResult is a per-other-user, per-path access result.
type CrossUserResult struct {
	OtherUser string `json:"other_user"`
	Path      string `json:"path"`
	Status    string `json:"status"` // missing | denied | leaked | unknown-user
}

// MarshalJSON ensures a stable key order for golden-file tests by
// delegating to a named alias. The custom marshaler is unnecessary at
// runtime; it exists so the doctor subcommand can emit human-friendly
// pretty output without pulling in a sort library.
func (r IsolationReport) PrettyJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// AuditConfig bundles the inputs to RunIsolationProbe.
type AuditConfig struct {
	// Project is the cc-connect project name, copied into the report.
	Project string
	// RunAsUser is the target user to spawn the probe as.
	RunAsUser string
	// WorkDir is the project's work_dir, passed to the probe.
	WorkDir string
	// OtherUsers is the list of other run_as_user values configured in
	// the same cc-connect instance, used for cross-user denial tests.
	OtherUsers []string
	// Supervisor is the supervisor Unix username (used for the
	// supervisor-denial leg of the probe). Usually derived from
	// os/user.Current.
	Supervisor string
	// Runner is the SudoRunner used to invoke the probe. Tests inject
	// stubs; production uses ExecSudoRunner.
	Runner SudoRunner
	// ProbeScriptOverride, if non-nil, replaces the embedded probe
	// script. Tests use this; production always uses the embedded one.
	ProbeScriptOverride []byte
	// Timeout bounds the probe invocation.
	Timeout time.Duration
}

// RunIsolationProbe spawns the probe as the target user and parses its
// output. Network effects: one sudo exec. Does not fail on non-zero exit
// from the probe — we parse whatever was printed.
func RunIsolationProbe(ctx context.Context, cfg AuditConfig) (IsolationReport, error) {
	report := IsolationReport{
		Project:   cfg.Project,
		RunAsUser: cfg.RunAsUser,
		WorkDir:   cfg.WorkDir,
		Timestamp: time.Now().UTC(),
	}
	if cfg.RunAsUser == "" {
		return report, errors.New("RunIsolationProbe: RunAsUser is empty")
	}
	if cfg.Runner == nil {
		cfg.Runner = ExecSudoRunner{}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	script := cfg.ProbeScriptOverride
	if script == nil {
		script = runasProbeScript
	}

	probeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Build env injection: since sudo -i strips env, we pass the probe
	// inputs as SHELL VARIABLES by prepending `export` statements to the
	// script body. Values are pre-validated at config parse time so
	// shell-quoting concerns are limited, but we still quote everything.
	header := fmt.Sprintf(
		"export CC_PROBE_WORKDIR=%s\nexport CC_PROBE_OTHER_USERS=%s\nexport CC_PROBE_SUPERVISOR=%s\n",
		shellQuote(cfg.WorkDir),
		shellQuote(strings.Join(filterOtherUsers(cfg.OtherUsers, cfg.RunAsUser), " ")),
		shellQuote(cfg.Supervisor),
	)
	fullScript := append([]byte(header), script...)

	// We invoke `sudo -n -iu <user> -- /bin/sh -s` and pipe the script on
	// stdin. Using -s + stdin avoids argv-length limits and avoids ever
	// putting the script body on the command line.
	cmd := exec.CommandContext(probeCtx, "sudo",
		"-n", "-iu", cfg.RunAsUser, "--", "/bin/sh", "-s")
	cmd.Stdin = bytes.NewReader(fullScript)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Still try to parse anything that made it out. Return the err
		// so callers can tell the probe didn't complete cleanly.
		report.RawOutput = stdout.String()
		parseProbeOutput(&report, stdout.String())
		return report, fmt.Errorf("probe exec failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	report.RawOutput = stdout.String()
	parseProbeOutput(&report, stdout.String())
	report.Fatal = computeAuditFatal(report)
	return report, nil
}

// parseProbeOutput walks the probe's line-oriented output and fills the
// report in place. Unknown tags are ignored (forward compatibility).
func parseProbeOutput(report *IsolationReport, out string) {
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		tag, rest := splitTag(line)
		switch tag {
		case "BEGIN":
			if strings.HasPrefix(rest, "probe-version=") {
				report.ProbeVersion = strings.TrimPrefix(rest, "probe-version=")
			}
		case "END":
			// no-op
		case "ID":
			report.Identity.ID = rest
		case "WHOAMI":
			report.Identity.Whoami = rest
		case "GROUPS":
			report.Identity.Groups = rest
		case "UMASK":
			report.Identity.Umask = rest
		case "PWD":
			report.Identity.Pwd = rest
		case "HOME":
			report.Identity.Home = rest
		case "SHELL":
			report.Identity.Shell = rest
		case "WORKDIR_PATH":
			report.WorkDirStatus.Path = rest
		case "WORKDIR_EXISTS":
			report.WorkDirStatus.Exists = rest == "yes"
		case "WORKDIR_READABLE":
			report.WorkDirStatus.Readable = rest == "yes"
		case "WORKDIR_WRITABLE":
			report.WorkDirStatus.Writable = rest == "yes"
		case "TARGET_HAS":
			report.TargetPaths = append(report.TargetPaths, PathStatus{Path: rest, Status: "has"})
		case "TARGET_MISSING":
			report.TargetPaths = append(report.TargetPaths, PathStatus{Path: rest, Status: "missing"})
		case "CROSS_DENIED", "CROSS_LEAKED", "CROSS_MISSING", "CROSS_UNKNOWN":
			other, path := splitTag(rest)
			status := strings.ToLower(strings.TrimPrefix(tag, "CROSS_"))
			if tag == "CROSS_UNKNOWN" {
				status = "unknown-user"
				other = rest
				path = ""
			}
			report.CrossUser = append(report.CrossUser, CrossUserResult{
				OtherUser: other,
				Path:      path,
				Status:    status,
			})
		case "SUPERVISOR_DENIED":
			report.Supervisor = append(report.Supervisor, PathStatus{Path: rest, Status: "denied"})
		case "SUPERVISOR_LEAKED":
			report.Supervisor = append(report.Supervisor, PathStatus{Path: rest, Status: "leaked"})
		case "SUPERVISOR_MISSING":
			report.Supervisor = append(report.Supervisor, PathStatus{Path: rest, Status: "missing"})
		}
	}
}

// computeAuditFatal applies the failure policy to a parsed report.
func computeAuditFatal(r IsolationReport) []string {
	var fatal []string
	for _, c := range r.CrossUser {
		if c.Status == "leaked" {
			fatal = append(fatal, fmt.Sprintf(
				"project %q: target user %q can read %q belonging to user %q (CROSS_LEAKED)",
				r.Project, r.RunAsUser, c.Path, c.OtherUser))
		}
	}
	for _, s := range r.Supervisor {
		if s.Status == "leaked" {
			fatal = append(fatal, fmt.Sprintf(
				"project %q: target user %q can read supervisor path %q (SUPERVISOR_LEAKED)",
				r.Project, r.RunAsUser, s.Path))
		}
	}
	if r.WorkDirStatus.Path != "" && !r.WorkDirStatus.Writable {
		fatal = append(fatal, fmt.Sprintf(
			"project %q: target user %q cannot write work_dir %q (WORKDIR_WRITABLE=no)",
			r.Project, r.RunAsUser, r.WorkDirStatus.Path))
	}
	return fatal
}

// splitTag splits "TAG rest of line" into (tag, rest). If there is no
// space, the whole string is the tag and rest is "".
func splitTag(line string) (string, string) {
	sp := strings.IndexByte(line, ' ')
	if sp < 0 {
		return line, ""
	}
	return line[:sp], line[sp+1:]
}

// shellQuote returns s wrapped in single quotes, with any existing single
// quotes escaped. Safe for POSIX sh.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// filterOtherUsers removes self and empty entries.
func filterOtherUsers(others []string, self string) []string {
	out := make([]string, 0, len(others))
	for _, o := range others {
		if o == "" || o == self {
			continue
		}
		out = append(out, o)
	}
	return out
}
