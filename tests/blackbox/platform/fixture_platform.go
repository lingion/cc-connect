//go:build blackbox

package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/core"
	"github.com/chenhg5/cc-connect/tests/blackbox/collector"
)

// FixturePlatform replays saved collector.Fixture files through the engine.
// It is backed by a MockPlatform, so all the standard assertion helpers work.
//
// Usage:
//
//	fp := platform.NewFixturePlatform(t, "feishu", "tests/blackbox/fixtures/feishu")
//	engine, _ := core.NewEngine(core.EngineConfig{Platforms: []core.Platform{fp}})
//	engine.Start()
//	fp.ReplayFile("20260101_text_hello.json")
//	reply := fp.WaitForTurnComplete(60 * time.Second)
type FixturePlatform struct {
	*MockPlatform
	t          *testing.T
	fixtureDir string
}

// NewFixturePlatform creates a FixturePlatform that reads fixtures from fixtureDir.
// fixtureDir may be a relative path; it is resolved relative to the repo root.
func NewFixturePlatform(t *testing.T, platformName, fixtureDir string) *FixturePlatform {
	t.Helper()
	mp := New(platformName + "-fixture")
	return &FixturePlatform{
		MockPlatform: mp,
		t:            t,
		fixtureDir:   fixtureDir,
	}
}

// ReplayFile injects the message from a single fixture file (relative to fixtureDir).
func (fp *FixturePlatform) ReplayFile(fileName string) {
	fp.t.Helper()
	path := filepath.Join(fp.fixtureDir, fileName)
	if err := fp.replayFile(path); err != nil {
		fp.t.Fatalf("fixture replay %q: %v", fileName, err)
	}
}

// ReplayAll injects all fixture files in fixtureDir whose names contain filter.
// Pass "" for filter to replay everything.
func (fp *FixturePlatform) ReplayAll(filter string) int {
	fp.t.Helper()
	entries, err := os.ReadDir(fp.fixtureDir)
	if err != nil {
		fp.t.Fatalf("fixture dir %q: %v", fp.fixtureDir, err)
	}

	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if filter == "" || strings.Contains(e.Name(), filter) {
			paths = append(paths, filepath.Join(fp.fixtureDir, e.Name()))
		}
	}
	sort.Strings(paths)

	for _, p := range paths {
		if err := fp.replayFile(p); err != nil {
			fp.t.Errorf("fixture replay %q: %v", p, err)
		}
	}
	return len(paths)
}

// LoadFixture reads and returns a fixture without injecting it.
func LoadFixture(path string) (*collector.Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f collector.Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("unmarshal %q: %w", path, err)
	}
	return &f, nil
}

// replayFile reads a Fixture JSON file and injects it into the engine.
func (fp *FixturePlatform) replayFile(path string) error {
	f, err := LoadFixture(path)
	if err != nil {
		return err
	}
	fp.t.Logf("fixture replay: %s  type=%s  content=%q",
		filepath.Base(path), f.MessageType, truncate(f.Message.Content, 60))

	msg := fixtureToMessage(f)
	fp.InjectRawMessage(msg)
	return nil
}

// fixtureToMessage converts a Fixture back to a core.Message suitable for
// injection.  Raw attachment bytes are not available from fixtures; only
// metadata is preserved.  Tests that need real bytes should use
// InjectMessageWithAttachments directly.
func fixtureToMessage(f *collector.Fixture) *core.Message {
	fm := f.Message
	return &core.Message{
		Platform:   fm.Platform,
		SessionKey: fm.SessionKey,
		MessageID:  fm.MessageID + "_replay_" + fmt.Sprint(time.Now().UnixNano()),
		UserID:     fm.UserID,
		UserName:   fm.UserName,
		ChatName:   fm.ChatName,
		Content:    fm.Content,
		ReplyCtx:   &MockReplyCtx{Platform: fm.Platform, ChatID: extractChatID(fm.SessionKey), UserID: fm.UserID},
	}
}

// FixtureToMessage converts a collector.Fixture to a *core.Message for injection.
// Raw attachment bytes are unavailable from sanitised fixtures; tests that need
// actual bytes should use InjectMessageWithAttachments directly.
func FixtureToMessage(f *collector.Fixture) *core.Message {
	return fixtureToMessage(f)
}

// extractChatID parses the middle segment of "platform:chatID:userID".
func extractChatID(sessionKey string) string {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return sessionKey
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
