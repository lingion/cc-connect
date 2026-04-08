package main

import (
	"strings"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestDefaultAuditDir_HomeSuffix(t *testing.T) {
	dir, err := defaultAuditDir()
	if err != nil {
		t.Fatalf("defaultAuditDir error: %v", err)
	}
	if !strings.HasSuffix(dir, "/.cc-connect/audits") {
		t.Errorf("audit dir = %q, want suffix /.cc-connect/audits", dir)
	}
}

// captureStdout swaps os.Stdout for the duration of fn and returns the
// captured output.
func TestPrintHumanReport_DoesNotPanic(t *testing.T) {
	// Build a populated report with a cross-user leak to exercise the
	// fatal path.
	r := core.IsolationReport{
		Project:   "demo",
		RunAsUser: "coder",
	}
	r.Identity.Whoami = "coder"
	r.Identity.ID = "uid=1001(coder)"
	r.Identity.Home = "/home/coder"
	r.WorkDirStatus.Path = "/tmp/wd"
	r.WorkDirStatus.Readable = true
	r.WorkDirStatus.Writable = true
	r.TargetPaths = []core.PathStatus{
		{Path: "/home/coder/.claude/settings.json", Status: "has"},
		{Path: "/home/coder/.pgpass", Status: "missing"},
	}
	r.CrossUser = []core.CrossUserResult{
		{OtherUser: "leigh", Path: "/home/leigh/.pgpass", Status: "leaked"},
	}
	r.Fatal = []string{"cross-user leak"}

	// Just make sure it doesn't panic and touches all branches.
	printHumanReport(r)
}
