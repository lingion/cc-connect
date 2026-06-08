package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ── Recording helpers ──────────────────────────────────────────

type recordingAgentSessionSend struct {
	stubAgentSession
	mu       sync.Mutex
	prompts  []string
	images   [][]ImageAttachment
	files    [][]FileAttachment
	aliveVal bool
}

func (s *recordingAgentSessionSend) Send(prompt string, images []ImageAttachment, files []FileAttachment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	imgs := append([]ImageAttachment(nil), images...)
	fls := append([]FileAttachment(nil), files...)
	s.images = append(s.images, imgs)
	s.files = append(s.files, fls)
	return nil
}

func (s *recordingAgentSessionSend) Alive() bool { return s.aliveVal }

func (s *recordingAgentSessionSend) promptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.prompts)
}

func (s *recordingAgentSessionSend) lastPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.prompts) == 0 {
		return ""
	}
	return s.prompts[len(s.prompts)-1]
}

type sendTrackingPlatform struct {
	stubPlatformEngine
	mu        sync.Mutex
	sendCalls []string
	replyCalls []string
}

func (p *sendTrackingPlatform) Send(ctx context.Context, replyCtx any, content string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sendCalls = append(p.sendCalls, content)
	return nil
}

func (p *sendTrackingPlatform) Reply(ctx context.Context, replyCtx any, content string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.replyCalls = append(p.replyCalls, content)
	return nil
}

func (p *sendTrackingPlatform) sendCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sendCalls)
}

func (p *sendTrackingPlatform) replyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.replyCalls)
}

// sendAgent wraps a single AgentSession returned by StartSession.
type sendAgent struct {
	session AgentSession
}

func (a *sendAgent) Name() string { return "send-agent" }
func (a *sendAgent) StartSession(_ context.Context, _ string) (AgentSession, error) {
	return a.session, nil
}
func (a *sendAgent) ListSessions(_ context.Context) ([]AgentSessionInfo, error) { return nil, nil }
func (a *sendAgent) Stop() error                                                { return nil }

func newSendEngine(t *testing.T, sess AgentSession, p Platform) *Engine {
	t.Helper()
	e := NewEngine("test", &sendAgent{session: sess}, []Platform{p}, "", LangEnglish)
	e.interactiveStates["test:session-1"] = &interactiveState{
		agentSession: sess,
		platform:     p,
		replyCtx:     "rctx-1",
	}
	return e
}

func postSend(t *testing.T, api *APIServer, body SendRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	api.handleSend(rec, req)
	return rec
}

// ── Engine-level tests ─────────────────────────────────────────

func TestEngineInjectPrompt_DeliversToAgentSession(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)

	if err := e.InjectPrompt("test:session-1", "hello agent", nil, nil); err != nil {
		t.Fatalf("InjectPrompt: %v", err)
	}
	if sess.promptCount() != 1 {
		t.Fatalf("agent session Send call count = %d, want 1", sess.promptCount())
	}
	if got := sess.lastPrompt(); got != "hello agent" {
		t.Errorf("prompt = %q, want %q", got, "hello agent")
	}
	// Platform should NOT receive the message — that's the whole point of
	// the as-prompt path.
	if plat.sendCount() != 0 {
		t.Errorf("platform Send called %d times, want 0", plat.sendCount())
	}
	if plat.replyCount() != 0 {
		t.Errorf("platform Reply called %d times, want 0", plat.replyCount())
	}
}

func TestEngineInjectPrompt_NoSession(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)

	if err := e.InjectPrompt("test:nonexistent", "hello", nil, nil); err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
	if sess.promptCount() != 0 {
		t.Errorf("agent session should not have been called, got %d", sess.promptCount())
	}
}

func TestEngineInjectPrompt_DeadSession(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: false}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)

	if err := e.InjectPrompt("test:session-1", "hello", nil, nil); err == nil {
		t.Fatal("expected error for dead session, got nil")
	}
}

func TestEnginePostToNewThread_CallsPlatformSend(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)

	if err := e.PostToNewThread("test:session-1", "build completed"); err != nil {
		t.Fatalf("PostToNewThread: %v", err)
	}
	if plat.sendCount() != 1 {
		t.Errorf("platform Send call count = %d, want 1", plat.sendCount())
	}
	if plat.replyCount() != 0 {
		t.Errorf("platform Reply should NOT be called (would go in thread), got %d", plat.replyCount())
	}
	// Agent session should not receive the message — only the platform sees it.
	if sess.promptCount() != 0 {
		t.Errorf("agent session should not have been called, got %d", sess.promptCount())
	}
}

func TestEngineInjectPromptToNewThread_BothPathsFire(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)

	if err := e.InjectPromptToNewThread("test:session-1", "watcher tick"); err != nil {
		t.Fatalf("InjectPromptToNewThread: %v", err)
	}
	if plat.sendCount() != 1 {
		t.Errorf("platform Send call count = %d, want 1", plat.sendCount())
	}
	if sess.promptCount() != 1 {
		t.Errorf("agent session Send call count = %d, want 1", sess.promptCount())
	}
	if got := sess.lastPrompt(); got != "watcher tick" {
		t.Errorf("agent prompt = %q, want %q", got, "watcher tick")
	}
}

// ── API-level dispatch tests (issue #590 regression) ──────────

func TestHandleSend_DispatchesAsPrompt(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)
	api := &APIServer{engines: map[string]*Engine{"test": e}}

	rec := postSend(t, api, SendRequest{
		Project:    "test",
		SessionKey: "test:session-1",
		Message:    "investigate build",
		AsPrompt:   true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if sess.promptCount() != 1 {
		t.Errorf("agent Send count = %d, want 1", sess.promptCount())
	}
	if plat.sendCount() != 0 {
		t.Errorf("platform Send count = %d, want 0 (as-prompt should NOT post to platform)", plat.sendCount())
	}
}

func TestHandleSend_DispatchesNewThread(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)
	api := &APIServer{engines: map[string]*Engine{"test": e}}

	rec := postSend(t, api, SendRequest{
		Project:    "test",
		SessionKey: "test:session-1",
		Message:    "build completed",
		NewThread:  true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if plat.sendCount() != 1 {
		t.Errorf("platform Send count = %d, want 1", plat.sendCount())
	}
	if plat.replyCount() != 0 {
		t.Errorf("platform Reply count = %d, want 0 (new-thread uses Send, not Reply)", plat.replyCount())
	}
	if sess.promptCount() != 0 {
		t.Errorf("agent Send count = %d, want 0 (new-thread alone does NOT inject)", sess.promptCount())
	}
}

// TestHandleSend_DispatchesAsPromptAndNewThread is the regression for
// issue #590: previously the handleSend returned early after the AsPrompt
// branch, so --as-prompt --new-thread silently dropped the new-thread
// behavior. With the fix, BOTH branches must fire.
func TestHandleSend_DispatchesAsPromptAndNewThread(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)
	api := &APIServer{engines: map[string]*Engine{"test": e}}

	rec := postSend(t, api, SendRequest{
		Project:    "test",
		SessionKey: "test:session-1",
		Message:    "completion event",
		AsPrompt:   true,
		NewThread:  true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	// BOTH must happen — the whole point of the combined mode.
	if plat.sendCount() != 1 {
		t.Errorf("platform Send count = %d, want 1 (new-thread)", plat.sendCount())
	}
	if sess.promptCount() != 1 {
		t.Errorf("agent Send count = %d, want 1 (as-prompt)", sess.promptCount())
	}
	if got := sess.lastPrompt(); got != "completion event" {
		t.Errorf("agent prompt = %q, want %q", got, "completion event")
	}
}

func TestHandleSend_NoFlagsFallsThroughToLegacy(t *testing.T) {
	sess := &recordingAgentSessionSend{aliveVal: true}
	plat := &sendTrackingPlatform{stubPlatformEngine: stubPlatformEngine{n: "test"}}
	e := newSendEngine(t, sess, plat)
	api := &APIServer{engines: map[string]*Engine{"test": e}}

	rec := postSend(t, api, SendRequest{
		Project:    "test",
		SessionKey: "test:session-1",
		Message:    "plain message",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	// Default path uses platform Send (legacy behavior).
	if plat.sendCount() != 1 {
		t.Errorf("platform Send count = %d, want 1 (legacy)", plat.sendCount())
	}
	// No prompt injection.
	if sess.promptCount() != 0 {
		t.Errorf("agent Send count = %d, want 0 (no flags means no injection)", sess.promptCount())
	}
}
