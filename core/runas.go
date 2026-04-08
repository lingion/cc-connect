//go:build !windows

// Package core — runas.go provides the spawn-as-different-Unix-user primitive
// used when a project sets `run_as_user` in config.toml.
//
// # Mechanism
//
// We intentionally spawn via:
//
//	sudo -n -iu <target-user> -- <command> [args...]
//
// The flags are load-bearing and should NOT be "simplified":
//
//   - -n (non-interactive): never prompt for a password. If passwordless
//     sudo to the target user is not configured, fail loudly instead of
//     hanging on a prompt that nobody will ever see.
//
//   - -i (simulate initial login): run the target user's full login shell,
//     loading their ~/.profile / ~/.bashrc, setting HOME to their home
//     directory, and clearing the supervisor's environment. This is what
//     makes the spawned process a "real session as that user" — their
//     ~/.claude/settings.json, their PGSSL certs, their plugin state.
//
//   - -u <target-user>: the target uid. Must be a specific username; the
//     sudoers rule that allows this should be scoped to this user only,
//     not ALL.
//
//   - -- : end of sudo options. Everything after this is the command to run
//     as the target user. Prevents an argv element that starts with "-"
//     from being reinterpreted as a sudo flag.
//
// Alternatives that are NOT used, with reasons:
//
//   - setuid(): loses the target user's shell profile entirely. No
//     ~/.bashrc, no ~/.profile, no login env. Also has to be done before
//     exec, which means the supervisor process needs CAP_SETUID or to be
//     running as root — strictly worse than sudo on both fronts.
//
//   - su - <target>: interactive-only on many distros (no -c equivalent
//     for a non-shell argv), and it consults PAM differently from sudo,
//     making the "passwordless" surface harder to reason about.
//
//   - sudo -u <target> (without -i): preserves the supervisor's cwd and
//     most of its environment. This leaks the supervisor's HOME and any
//     unset-by-default env vars, which defeats the isolation story.
//
// # Environment handling
//
// When RunAsUser is set, the supervisor's environment is NOT forwarded to
// the target user. Only variables on the explicit allowlist are passed
// through via `sudo --preserve-env=VAR1,VAR2`. The default allowlist is
// intentionally minimal; anything else should live in the target user's
// own shell profile or ~/.claude/settings.json.
package core

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// DefaultEnvAllowlist is the minimal set of environment variables that are
// preserved across the sudo boundary when RunAsUser is set. These are the
// ones where "use the target user's value" either makes no sense (TERM) or
// would break tooling that doesn't read them from the login profile (PATH
// may be set by /etc/environment and the target's profile; we let sudo -i
// override it if the target's profile sets its own).
//
// Deliberately excluded: HOME (overridden by -i), USER (overridden by -i),
// LOGNAME (overridden by -i), SHELL (overridden by -i), PWD (set by cmd.Dir),
// anything project-specific or secret.
var DefaultEnvAllowlist = []string{
	"PATH",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"LC_MESSAGES",
	"TERM",
}

// SpawnOptions controls how a command is spawned. Zero value means
// "supervisor user, legacy behavior".
type SpawnOptions struct {
	// RunAsUser, when non-empty, causes the command to be spawned via
	// `sudo -n -iu RunAsUser -- ...`. Empty = legacy behavior.
	RunAsUser string

	// EnvAllowlist extends DefaultEnvAllowlist with additional variable
	// names that should cross the sudo boundary. The union of both lists
	// is passed to `sudo --preserve-env=...`.
	EnvAllowlist []string
}

// IsolationMode reports whether the options request OS-user isolation.
func (o SpawnOptions) IsolationMode() bool {
	return o.RunAsUser != ""
}

// mergedAllowlist returns the sorted, deduplicated union of
// DefaultEnvAllowlist and o.EnvAllowlist.
func (o SpawnOptions) mergedAllowlist() []string {
	seen := make(map[string]struct{}, len(DefaultEnvAllowlist)+len(o.EnvAllowlist))
	for _, v := range DefaultEnvAllowlist {
		seen[v] = struct{}{}
	}
	for _, v := range o.EnvAllowlist {
		if v == "" {
			continue
		}
		seen[v] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// BuildSpawnCommand constructs an *exec.Cmd that, depending on opts, either
// invokes name/args directly (legacy) or wraps them in
// `sudo -n -iu <user> --preserve-env=<allowlist> -- name args...`.
//
// Callers are responsible for setting cmd.Dir, cmd.Env (via
// FilterEnvForSpawn), cmd.Stdin/Stdout/Stderr, and cmd.SysProcAttr before
// calling Start().
//
// BuildSpawnCommand does NOT perform the per-spawn re-check — see
// VerifyRunAsUserCheap. Callers should run the re-check immediately before
// Start() so that a sudoers change between preflight and spawn is caught.
func BuildSpawnCommand(ctx context.Context, opts SpawnOptions, name string, args ...string) *exec.Cmd {
	if !opts.IsolationMode() {
		return exec.CommandContext(ctx, name, args...)
	}
	sudoArgs := []string{
		"-n",                          // never prompt
		"-iu", opts.RunAsUser,         // initial-login as target
		"--preserve-env=" + strings.Join(opts.mergedAllowlist(), ","),
		"--",                          // end of sudo flags
		name,
	}
	sudoArgs = append(sudoArgs, args...)
	return exec.CommandContext(ctx, "sudo", sudoArgs...)
}

// FilterEnvForSpawn returns a copy of env containing only variables whose
// names appear in the merged allowlist (DefaultEnvAllowlist ∪
// opts.EnvAllowlist). When opts.RunAsUser is empty, env is returned
// unchanged.
//
// This is belt-and-braces with `sudo --preserve-env=...`: sudo already
// strips anything not in its preserve list, but clearing cmd.Env here
// makes the spawn command's own argv the single source of truth for what
// crosses the boundary, and keeps test assertions simple.
func FilterEnvForSpawn(env []string, opts SpawnOptions) []string {
	if !opts.IsolationMode() {
		return env
	}
	allow := opts.mergedAllowlist()
	allowSet := make(map[string]struct{}, len(allow))
	for _, v := range allow {
		allowSet[v] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			continue
		}
		if _, ok := allowSet[e[:eq]]; ok {
			out = append(out, e)
		}
	}
	return out
}

// SudoRunner is an injectable interface for running sudo commands. The
// production implementation calls exec.CommandContext; tests inject a stub
// that returns canned results without touching the real sudo binary.
type SudoRunner interface {
	// Run executes `sudo <args...>` and returns combined stdout+stderr.
	// The exit code is encoded in err as *exec.ExitError; nil means exit 0.
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecSudoRunner is the production SudoRunner backed by os/exec.
type ExecSudoRunner struct{}

// Run implements SudoRunner.
func (ExecSudoRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "sudo", args...).CombinedOutput()
}

// VerifyRunAsUserCheap runs the two cheap preflight checks that must pass
// before every spawn, not just at startup:
//
//  1. `sudo -n -iu <user> -- /bin/true` must succeed — the supervisor still
//     has passwordless sudo to the target user.
//  2. `sudo -n -iu <user> -- sudo -n /bin/true` must FAIL — the target user
//     cannot non-interactively escalate.
//
// Returns nil if both checks behave as expected. Returns a descriptive
// error otherwise. This is intentionally fast: no filesystem walks, no
// JSON, just two exec calls. Meant to be safe to run on every spawn.
//
// The expensive checks (work_dir access, isolation probe) live in the
// preflight and audit packages and only run at startup / via `cc-connect
// doctor user-isolation`.
func VerifyRunAsUserCheap(ctx context.Context, runner SudoRunner, runAsUser string) error {
	if runAsUser == "" {
		return errors.New("VerifyRunAsUserCheap: runAsUser is empty")
	}
	// Check 1: passwordless sudo to target works.
	if out, err := runner.Run(ctx, "-n", "-iu", runAsUser, "--", "/bin/true"); err != nil {
		return fmt.Errorf("passwordless sudo to user %q failed (check that your sudoers rule is present and scoped to this user): %w: %s", runAsUser, err, strings.TrimSpace(string(out)))
	}
	// Check 2: target cannot escalate via sudo.
	out, err := runner.Run(ctx, "-n", "-iu", runAsUser, "--", "sudo", "-n", "/bin/true")
	if err == nil {
		return fmt.Errorf("target user %q can run passwordless sudo; isolation is meaningless. Remove NOPASSWD sudo for this user. Output: %s", runAsUser, strings.TrimSpace(string(out)))
	}
	return nil
}
