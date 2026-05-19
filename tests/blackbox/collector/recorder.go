// Package collector provides fixture recording for blackbox platform tests.
//
// When CC_CONNECT_RECORD_FIXTURES=/path is set in the environment, wrapping a
// platform with [Recorder.Wrap] causes every incoming core.Message to be
// serialised (without raw bytes) to a JSON file under that path.
//
// Typical workflow for collecting fixtures from the live supervisor service:
//
//  1. Set the env var and restart cc-connect:
//       CC_CONNECT_RECORD_FIXTURES=/tmp/cc-fixtures supervisorctl restart cc-connect
//
//  2. Trigger messages on each platform (see docs/FIXTURE-COLLECTION.md).
//
//  3. Stop recording and copy fixtures to the test tree:
//       cp -r /tmp/cc-fixtures/ tests/blackbox/fixtures/
//
//  4. Sanitise PII using [SanitizeFile] or the cli helper (optional if already
//     using [Recorder] which redacts IDs automatically).
package collector

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/chenhg5/cc-connect/core"
)

// Recorder intercepts core.Message objects and writes them to disk as JSON fixtures.
type Recorder struct {
	dir     string
	counter atomic.Int64
}

// New creates a Recorder that saves fixtures to dir (created on demand).
func New(dir string) *Recorder {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("collector: cannot create fixture dir", "dir", dir, "err", err)
	}
	return &Recorder{dir: dir}
}

// FromEnv returns a non-nil Recorder when CC_CONNECT_RECORD_FIXTURES is set.
func FromEnv() *Recorder {
	if dir := os.Getenv("CC_CONNECT_RECORD_FIXTURES"); dir != "" {
		return New(dir)
	}
	return nil
}

// Wrap returns a platform that records every incoming core.Message before
// passing it to the engine.  If r is nil, Wrap returns p unchanged.
func (r *Recorder) Wrap(p core.Platform) core.Platform {
	if r == nil {
		return p
	}
	return &recordingPlatform{Platform: p, rec: r}
}

// recordingPlatform intercepts the Start handler to record messages.
type recordingPlatform struct {
	core.Platform
	rec *Recorder
}

func (rp *recordingPlatform) Start(handler core.MessageHandler) error {
	return rp.Platform.Start(func(p core.Platform, msg *core.Message) {
		if msg != nil && !msg.Recalled {
			rp.rec.record(p.Name(), msg)
		}
		handler(p, msg)
	})
}

// record serialises msg to a JSON fixture file in dir/<platform>/.
func (r *Recorder) record(platformName string, msg *core.Message) {
	n := r.counter.Add(1)
	safe := safeName(platformName)

	subDir := filepath.Join(r.dir, safe)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		slog.Warn("collector: mkdir failed", "err", err)
		return
	}

	ts := time.Now().Format("20060102_150405")
	msgType := classifyMessage(msg)
	tail := sanitizeID(msg.MessageID)
	name := fmt.Sprintf("%s_%03d_%s_%s.json", ts, n, msgType, tail)
	path := filepath.Join(subDir, name)

	fixture := Fixture{
		SchemaVersion: 1,
		CapturedAt:    time.Now().UTC(),
		Platform:      platformName,
		MessageType:   msgType,
		Message:       sanitize(msg),
	}

	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		slog.Warn("collector: marshal failed", "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		slog.Warn("collector: write failed", "path", path, "err", err)
		return
	}

	slog.Info("collector: fixture saved",
		"path", path, "platform", platformName, "type", msgType,
		"msg_id", msg.MessageID, "user", msg.UserName)
}

// ─── on-disk types ────────────────────────────────────────────────────────────

// Fixture is the on-disk JSON format for a captured message.
type Fixture struct {
	SchemaVersion int            `json:"schema_version"`
	CapturedAt    time.Time      `json:"captured_at"`
	Platform      string         `json:"platform"`
	MessageType   string         `json:"message_type"`
	Message       FixtureMessage `json:"message"`
}

// FixtureMessage is a sanitised snapshot of core.Message (no raw bytes or tokens).
type FixtureMessage struct {
	Platform   string `json:"platform"`
	SessionKey string `json:"session_key"`
	MessageID  string `json:"message_id"`
	ChatName   string `json:"chat_name,omitempty"`
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name,omitempty"`
	Content    string `json:"content"`

	// Attachment metadata (raw bytes are omitted to keep fixtures small).
	ImagesCount int        `json:"images_count,omitempty"`
	ImagesMeta  []ImageMeta `json:"images,omitempty"`
	FilesCount  int        `json:"files_count,omitempty"`
	FilesMeta   []FileMeta  `json:"files,omitempty"`
	HasAudio    bool       `json:"has_audio,omitempty"`

	// Derived hints used when building test assertions from this fixture.
	IsGroupChat bool   `json:"is_group_chat,omitempty"`
	IsMentioned bool   `json:"is_mentioned,omitempty"`
}

// ImageMeta records image metadata without raw bytes.
type ImageMeta struct {
	MIMEType string `json:"mime_type,omitempty"`
	Size     int    `json:"size_bytes,omitempty"`
}

// FileMeta records file metadata without raw bytes.
type FileMeta struct {
	FileName string `json:"file_name,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Size     int    `json:"size_bytes,omitempty"`
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// sanitize copies msg fields to FixtureMessage, redacting PII.
func sanitize(msg *core.Message) FixtureMessage {
	fm := FixtureMessage{
		Platform:    msg.Platform,
		SessionKey:  msg.SessionKey,
		MessageID:   msg.MessageID,
		ChatName:    msg.ChatName,
		UserID:      redactID(msg.UserID),
		UserName:    redactName(msg.UserName),
		Content:     msg.Content,
		HasAudio:    msg.Audio != nil,
		IsGroupChat: isGroupSession(msg.SessionKey),
		IsMentioned: strings.Contains(msg.Content, "@"),
	}

	fm.ImagesCount = len(msg.Images)
	for _, img := range msg.Images {
		fm.ImagesMeta = append(fm.ImagesMeta, ImageMeta{
			MIMEType: img.MimeType,
			Size:     len(img.Data),
		})
	}

	fm.FilesCount = len(msg.Files)
	for _, f := range msg.Files {
		fm.FilesMeta = append(fm.FilesMeta, FileMeta{
			FileName: f.FileName,
			MIMEType: f.MimeType,
			Size:     len(f.Data),
		})
	}

	return fm
}

// SanitizeFile reads a raw (unsanitised) fixture and writes a sanitised copy
// to outPath.  Useful as a standalone CLI step after manual collection.
func SanitizeFile(inPath, outPath string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	// Re-sanitize IDs (in case the file was written directly without recorder).
	f.Message.UserID = redactID(f.Message.UserID)
	f.Message.UserName = redactName(f.Message.UserName)

	out, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0o644)
}

func classifyMessage(msg *core.Message) string {
	switch {
	case len(msg.Images) > 0 && len(msg.Files) > 0:
		return "mixed"
	case len(msg.Images) > 0:
		return "image"
	case len(msg.Files) > 0:
		return "file"
	case msg.Audio != nil:
		return "audio"
	case msg.Content != "":
		return "text"
	default:
		return "unknown"
	}
}

func isGroupSession(sessionKey string) bool {
	// Common patterns: feishu:oc_xxx:user (group), feishu:ou_xxx:user (p2p)
	return strings.Contains(sessionKey, ":oc_") ||
		strings.Contains(sessionKey, "group") ||
		strings.Contains(sessionKey, "channel")
}

// redactID keeps only the first 4 chars and masks the rest.
func redactID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[:4] + strings.Repeat("*", len(id)-4)
}

// redactName keeps only the first token (first name).
func redactName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

func sanitizeID(id string) string {
	const max = 16
	if len(id) > max {
		id = id[len(id)-max:]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, id)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}
