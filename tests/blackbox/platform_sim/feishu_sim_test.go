//go:build blackbox

// Package platform_sim tests cc-connect using fixture-backed platform adapters.
//
// These tests exercise the full pipeline using messages captured from real
// platforms (see docs/FIXTURE-COLLECTION.md for how to collect them):
//
//	Fixture JSON → FixturePlatform → Engine → Real Agent → MockPlatform.Reply
//
// Run after collecting new fixtures:
//
//	go test -tags blackbox ./tests/blackbox/platform_sim/... -timeout 300s -v
package platform_sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bplatform "github.com/chenhg5/cc-connect/tests/blackbox/platform"
	"github.com/chenhg5/cc-connect/tests/blackbox/collector"
	"github.com/chenhg5/cc-connect/tests/blackbox/helper"
)

const feishuFixtureDir = "../fixtures/feishu"

// TestFeishuSim_TextMessage verifies that cc-connect correctly routes a
// text message captured from the live Feishu platform and that the agent
// replies within the timeout.
func TestFeishuSim_TextMessage(t *testing.T) {
	t.Parallel()
	path := requireFixture(t, feishuFixtureDir, "_text_")

	f := mustLoadFixture(t, path)
	t.Logf("replaying fixture: %s  content=%q", filepath.Base(path), truncate(f.Message.Content, 80))

	env := helper.NewEnv(t, "claudecode")
	before := env.Platform.MessageCount()
	env.Platform.InjectRawMessage(bplatform.FixtureToMessage(f))

	msgs := env.Platform.WaitForTurnComplete(before, 3*time.Second, 120*time.Second)
	if len(msgs) == 0 {
		t.Fatalf("feishu text sim: no reply within 120s")
	}
	t.Logf("reply: %q", truncate(msgs[len(msgs)-1].Text(), 120))
}

// TestFeishuSim_ImageMessage verifies image fixture routing.
func TestFeishuSim_ImageMessage(t *testing.T) {
	t.Parallel()
	path := requireFixture(t, feishuFixtureDir, "_image_")

	f := mustLoadFixture(t, path)
	if f.Message.ImagesCount == 0 {
		t.Skipf("fixture %s has 0 images – recollect with an image message", filepath.Base(path))
	}
	t.Logf("replaying image fixture: %s", filepath.Base(path))

	env := helper.NewEnv(t, "claudecode")
	before := env.Platform.MessageCount()
	env.Platform.InjectRawMessage(bplatform.FixtureToMessage(f))

	msgs := env.Platform.WaitForTurnComplete(before, 3*time.Second, 120*time.Second)
	if len(msgs) == 0 {
		t.Fatalf("feishu image sim: no reply within 120s")
	}
	t.Logf("reply: %q", truncate(msgs[len(msgs)-1].Text(), 120))
}

// TestFeishuSim_FileMessage verifies file fixture routing.
func TestFeishuSim_FileMessage(t *testing.T) {
	t.Parallel()
	path := requireFixture(t, feishuFixtureDir, "_file_")

	f := mustLoadFixture(t, path)
	if f.Message.FilesCount == 0 {
		t.Skipf("fixture %s has 0 files – recollect with a file message", filepath.Base(path))
	}
	t.Logf("replaying file fixture: %s  files=%v", filepath.Base(path), f.Message.FilesMeta)

	env := helper.NewEnv(t, "claudecode")
	before := env.Platform.MessageCount()
	env.Platform.InjectRawMessage(bplatform.FixtureToMessage(f))

	msgs := env.Platform.WaitForTurnComplete(before, 3*time.Second, 120*time.Second)
	if len(msgs) == 0 {
		t.Fatalf("feishu file sim: no reply within 120s")
	}
	t.Logf("reply: %q", truncate(msgs[len(msgs)-1].Text(), 120))
}

// TestFeishuSim_AllFixtures replays every collected fixture and verifies each
// one gets a reply.  This is the regression sweep: run after every SDK update.
func TestFeishuSim_AllFixtures(t *testing.T) {
	entries, err := os.ReadDir(feishuFixtureDir)
	if err != nil {
		t.Skipf("fixture dir not found – collect first: %v", err)
	}

	var fixtures []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") &&
			!strings.Contains(e.Name(), "sample_") {
			fixtures = append(fixtures, e.Name())
		}
	}
	if len(fixtures) == 0 {
		t.Skip("no real fixtures yet – run FIXTURE-COLLECTION.md first")
	}

	env := helper.NewEnv(t, "claudecode")

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := mustLoadFixture(t, filepath.Join(feishuFixtureDir, name))
			before := env.Platform.MessageCount()
			env.Platform.InjectRawMessage(bplatform.FixtureToMessage(f))
			msgs := env.Platform.WaitForTurnComplete(before, 3*time.Second, 90*time.Second)
			if len(msgs) == 0 {
				t.Errorf("fixture %s: no reply within 90s", name)
			}
		})
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// requireFixture finds the first fixture file containing filter in fixtureDir.
// It skips the test if none is found (so CI stays green without fixtures).
func requireFixture(t *testing.T, dir, filter string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("fixture dir %q not found – run FIXTURE-COLLECTION.md first", dir)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") &&
			!strings.Contains(e.Name(), "sample_") &&
			strings.Contains(e.Name(), filter) {
			return filepath.Join(dir, e.Name())
		}
	}
	t.Skipf("no fixture matching %q in %s – collect fixtures first", filter, dir)
	return ""
}

func mustLoadFixture(t *testing.T, path string) *collector.Fixture {
	t.Helper()
	f, err := bplatform.LoadFixture(path)
	if err != nil {
		t.Fatalf("load fixture %q: %v", path, err)
	}
	return f
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
