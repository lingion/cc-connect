//go:build !windows

package core

// runas_check.go — startup-time preflight gates for run_as_user.
//
// These are the hard go/no-go checks described in issue #496. They are
// intentionally more expensive than VerifyRunAsUserCheap (which only runs
// the two sudo probes) because they also touch the filesystem and walk the
// project's work_dir looking for permission problems the target user would
// hit at runtime.
//
// Use PreflightRunAsUser at cc-connect startup, in parallel across all
// projects, and refuse to start the daemon if any project returns any
// fatal error. Warnings are surfaced via slog but do not abort startup.
//
// Tests stub the SudoRunner so this file has no tie to an actual sudo
// binary.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PreflightResult captures the outcome of running the full preflight suite
// for a single project.
type PreflightResult struct {
	// Project is the config project name, used in log output.
	Project string
	// RunAsUser is the target user the checks were run for.
	RunAsUser string
	// Fatal errors, if any, mean cc-connect must not start this project.
	// If any project has any fatal, cc-connect aborts startup globally.
	Fatal []error
	// Warnings are non-fatal observations (e.g. descendant paths in
	// work_dir that the target user cannot read). They are logged but do
	// not block startup.
	Warnings []string
	// SudoListOutput captures `sudo -n -l` output collected when check 2
	// (target cannot escalate) fails, to help the operator find the
	// offending sudoers rule.
	SudoListOutput string
}

// HasFatal reports whether at least one fatal error was recorded.
func (r PreflightResult) HasFatal() bool { return len(r.Fatal) > 0 }

// DescendantScanConfig tunes the non-fatal work_dir descendant scan.
type DescendantScanConfig struct {
	// PrunePaths are directory basenames that are skipped during the
	// walk. Typical values: .git, node_modules, .venv.
	PrunePaths []string
	// MaxReport caps the number of individual path warnings printed
	// before summarizing the remainder as a count.
	MaxReport int
	// Timeout bounds the entire scan. If it elapses, the scan returns a
	// single "scan timed out" warning rather than partial results.
	Timeout time.Duration
}

// DefaultDescendantScanConfig is the baseline used unless a caller
// overrides it.
var DefaultDescendantScanConfig = DescendantScanConfig{
	PrunePaths: []string{
		".git", "node_modules", ".venv", "venv", "dist", "build",
		"target", ".pytest_cache", "__pycache__", ".next", ".cache",
	},
	MaxReport: 50,
	Timeout:   10 * time.Second,
}

// PreflightConfig bundles the inputs to PreflightRunAsUser so callers can
// pass a single struct rather than a long argument list.
type PreflightConfig struct {
	Project    string
	RunAsUser  string
	WorkDir    string
	Runner     SudoRunner
	ScanConfig DescendantScanConfig
}

// PreflightRunAsUser runs all three startup safety checks for a single
// project. It never panics and never returns nil; instead all problems are
// accumulated into the returned PreflightResult for the caller to aggregate
// and log.
//
// Checks:
//
//  1. Passwordless sudo -iu <target> is configured (fatal if missing).
//  2. Target user has no passwordless sudo (fatal if they can escalate);
//     on failure, captures `sudo -n -iu target -- sudo -n -l` output to
//     help the operator find the offending rule.
//  3. Target user can read AND write the work_dir root (fatal if not),
//     plus a best-effort descendant walk producing warnings for paths
//     the target user cannot access.
func PreflightRunAsUser(ctx context.Context, cfg PreflightConfig) PreflightResult {
	result := PreflightResult{Project: cfg.Project, RunAsUser: cfg.RunAsUser}
	if cfg.RunAsUser == "" {
		result.Fatal = append(result.Fatal, errors.New("PreflightRunAsUser: RunAsUser is empty"))
		return result
	}
	if cfg.Runner == nil {
		cfg.Runner = ExecSudoRunner{}
	}
	if cfg.ScanConfig.MaxReport == 0 {
		cfg.ScanConfig = DefaultDescendantScanConfig
	}

	// Check 1: passwordless sudo to target works.
	if _, err := cfg.Runner.Run(ctx, "-n", "-iu", cfg.RunAsUser, "--", "/bin/true"); err != nil {
		result.Fatal = append(result.Fatal, fmt.Errorf(
			"project %q: passwordless sudo to user %q is not configured. Add a sudoers rule such as:\n  %s ALL=(%s) NOPASSWD: ALL\nthen restart cc-connect. Underlying error: %w",
			cfg.Project, cfg.RunAsUser, currentUsernameOr("<supervisor>"), cfg.RunAsUser, err))
		return result // subsequent checks are pointless
	}

	// Check 2: target cannot escalate via sudo. Expected: command FAILS.
	if _, err := cfg.Runner.Run(ctx, "-n", "-iu", cfg.RunAsUser, "--", "sudo", "-n", "/bin/true"); err == nil {
		// Escalation succeeded — this is the failure case.
		// Collect sudo -l from the target's context for the error message.
		if out, listErr := cfg.Runner.Run(ctx, "-n", "-iu", cfg.RunAsUser, "--", "sudo", "-n", "-l"); listErr == nil {
			result.SudoListOutput = strings.TrimSpace(string(out))
		}
		msg := fmt.Sprintf(
			"project %q: target user %q can run passwordless sudo. The run_as_user sandbox provides no isolation if the spawned agent can escalate non-interactively. Remove NOPASSWD sudo access for this user before starting cc-connect.",
			cfg.Project, cfg.RunAsUser)
		if result.SudoListOutput != "" {
			msg += "\n\n`sudo -n -l` as " + cfg.RunAsUser + ":\n" + indent(result.SudoListOutput, "  ")
		}
		result.Fatal = append(result.Fatal, errors.New(msg))
		// Don't return early — still run check 3 so the operator gets all
		// the bad news in a single startup attempt.
	}

	// Check 3a (fatal): target user can read and write work_dir root.
	if cfg.WorkDir == "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"project %q: no work_dir configured; skipping filesystem access checks", cfg.Project))
	} else {
		absWorkDir := cfg.WorkDir
		if abs, err := filepath.Abs(absWorkDir); err == nil {
			absWorkDir = abs
		}
		if _, err := cfg.Runner.Run(ctx, "-n", "-iu", cfg.RunAsUser, "--", "test", "-r", absWorkDir, "-a", "-w", absWorkDir); err != nil {
			result.Fatal = append(result.Fatal, fmt.Errorf(
				"project %q: target user %q cannot read AND write work_dir %q. Agents will fail with EACCES at runtime. Fix ownership/permissions on this directory (chown/chmod or an ACL granting the target user rwx) before starting cc-connect.",
				cfg.Project, cfg.RunAsUser, absWorkDir))
		} else {
			// Check 3b (warning-only): walk descendants, collect paths
			// the target user cannot read/write. Runs with a timeout.
			warn := scanDescendants(ctx, cfg.Runner, cfg.RunAsUser, absWorkDir, cfg.ScanConfig)
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
			}
		}
	}

	return result
}

// scanDescendants runs a best-effort `find` as the target user under
// workDir and formats any access problems as a single warning string. It
// returns "" if no issues are found. Respects ScanConfig.Timeout.
func scanDescendants(ctx context.Context, runner SudoRunner, target, workDir string, scan DescendantScanConfig) string {
	scanCtx, cancel := context.WithTimeout(ctx, scan.Timeout)
	defer cancel()

	// Build a find expression that:
	//   - prunes noisy directories (-path .../name -prune -o ...)
	//   - prints only entries that the current user CANNOT read, cannot
	//     write, or (for dirs) cannot search.
	//
	// Using find's own permission tests means the check runs inside the
	// target user's context (because we invoke find via sudo -iu), so we
	// get the real answer without shelling out per-file.
	//
	// Output format per line: "MODE<TAB>PATH" where MODE is one of
	// "noread", "nowrite", "nosearch".
	var prune []string
	for _, p := range scan.PrunePaths {
		if len(prune) > 0 {
			prune = append(prune, "-o")
		}
		prune = append(prune, "-name", p)
	}
	// find <workDir> \( <prune exprs> \) -prune -o \( -not -readable -printf "noread\t%p\n" , -type f -not -writable -printf "nowrite\t%p\n" , -type d -not -executable -printf "nosearch\t%p\n" \) -print
	args := []string{
		"-n", "-iu", target, "--",
		"find", workDir,
	}
	if len(prune) > 0 {
		args = append(args, "(")
		args = append(args, prune...)
		args = append(args, ")", "-prune", "-o")
	}
	args = append(args,
		"(",
		"-not", "-readable", "-printf", `noread\t%p\n`,
		",",
		"-type", "f", "-not", "-writable", "-printf", `nowrite\t%p\n`,
		",",
		"-type", "d", "-not", "-executable", "-printf", `nosearch\t%p\n`,
		")",
	)

	out, err := runner.Run(scanCtx, args...)
	if scanCtx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("work_dir descendant scan timed out after %s (large repo?); skipping detailed access audit. Run `cc-connect doctor user-isolation` manually if you need it.", scan.Timeout)
	}
	// find exits non-zero if it couldn't stat some path. That's actually
	// data for us; we still parse whatever it printed.
	_ = err

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return ""
	}

	// Dedupe + sort for stable output.
	seen := make(map[string]struct{}, len(lines))
	var uniq []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		uniq = append(uniq, l)
	}
	sort.Strings(uniq)

	if len(uniq) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "work_dir %q contains paths that user %q may not access cleanly:\n", workDir, target)
	shown := 0
	for _, l := range uniq {
		if shown >= scan.MaxReport {
			break
		}
		fmt.Fprintf(&b, "  %s\n", l)
		shown++
	}
	if len(uniq) > scan.MaxReport {
		fmt.Fprintf(&b, "  ... and %d more\n", len(uniq)-scan.MaxReport)
	}
	b.WriteString("\nThe agent may fail with EACCES when accessing these paths. Fix ownership/permissions, narrow the project scope, or accept the risk if the inaccessible paths are intentionally out of bounds.")
	return b.String()
}

// currentUsernameOr returns the current Unix username or a fallback. Used
// purely for building the example sudoers line in error messages.
func currentUsernameOr(fallback string) string {
	if u := currentUsername(); u != "" {
		return u
	}
	return fallback
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
