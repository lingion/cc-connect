package acp

import (
	"encoding/json"
	"strings"

	"github.com/chenhg5/cc-connect/core"
)

// mapSessionUpdate turns one ACP session/update payload into zero or more core events.
func mapSessionUpdate(sessionID string, params json.RawMessage) []core.Event {
	var wrap struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(params, &wrap); err != nil || len(wrap.Update) == 0 {
		return nil
	}
	sid := wrap.SessionID
	if sid == "" {
		sid = sessionID
	}

	var head struct {
		SessionUpdate string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(wrap.Update, &head); err != nil {
		return nil
	}

	switch head.SessionUpdate {
	case "agent_message_chunk":
		return mapAgentMessageChunk(sid, wrap.Update)
	case "tool_call":
		return mapToolCall(sid, wrap.Update)
	case "tool_call_update":
		return mapToolCallUpdate(sid, wrap.Update)
	case "plan":
		return mapPlan(sid, wrap.Update)
	case "user_message_chunk":
		// History replay during session/load — show as text for IM.
		return mapAgentMessageChunk(sid, wrap.Update)
	default:
		return nil
	}
}

func mapAgentMessageChunk(sessionID string, update json.RawMessage) []core.Event {
	var u struct {
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(update, &u); err != nil {
		return nil
	}
	if u.Content.Text == "" {
		return nil
	}
	return []core.Event{{
		Type:      core.EventText,
		Content:   u.Content.Text,
		SessionID: sessionID,
	}}
}

func mapToolCall(sessionID string, update json.RawMessage) []core.Event {
	var u struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(update, &u); err != nil {
		return nil
	}
	toolName := u.Title
	if toolName == "" {
		toolName = u.Kind
	}
	if toolName == "" {
		toolName = "tool"
	}
	return []core.Event{{
		Type:      core.EventToolUse,
		ToolName:  toolName,
		ToolInput: u.Title,
		SessionID: sessionID,
	}}
}

func mapToolCallUpdate(sessionID string, update json.RawMessage) []core.Event {
	var u struct {
		ToolCallID string `json:"toolCallId"`
		Status     string `json:"status"`
		Content    []struct {
			Type    string `json:"type"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(update, &u); err != nil {
		return nil
	}
	if strings.EqualFold(u.Status, "completed") || strings.EqualFold(u.Status, "failed") {
		var b strings.Builder
		for _, c := range u.Content {
			if c.Content.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(c.Content.Text)
			}
		}
		name := u.ToolCallID
		if name == "" {
			name = "tool"
		}
		return []core.Event{{
			Type:      core.EventToolResult,
			ToolName:  name,
			Content:   truncateRunes(b.String(), 500),
			SessionID: sessionID,
		}}
	}
	return nil
}

func mapPlan(sessionID string, update json.RawMessage) []core.Event {
	var u struct {
		Entries []struct {
			Content  string `json:"content"`
			Priority string `json:"priority"`
			Status   string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(update, &u); err != nil || len(u.Entries) == 0 {
		return nil
	}
	var b strings.Builder
	for i, e := range u.Entries {
		if i > 0 {
			b.WriteByte('\n')
		}
		line := e.Content
		if e.Status != "" {
			line = "[" + e.Status + "] " + line
		}
		b.WriteString(line)
	}
	return []core.Event{{
		Type:      core.EventThinking,
		Content:   b.String(),
		SessionID: sessionID,
	}}
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

// permissionOption matches ACP session/request_permission option entries.
type permissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

func pickPermissionOptionID(allow bool, options []permissionOption) string {
	if len(options) == 0 {
		return ""
	}
	if allow {
		for _, o := range options {
			if strings.Contains(strings.ToLower(o.Kind), "allow") {
				return o.OptionID
			}
		}
		for _, o := range options {
			if strings.Contains(strings.ToLower(o.Name), "allow") {
				return o.OptionID
			}
		}
		return options[0].OptionID
	}
	for _, o := range options {
		if strings.Contains(strings.ToLower(o.Kind), "reject") || strings.Contains(strings.ToLower(o.Kind), "deny") {
			return o.OptionID
		}
	}
	for _, o := range options {
		if strings.Contains(strings.ToLower(o.Name), "reject") || strings.Contains(strings.ToLower(o.Name), "deny") {
			return o.OptionID
		}
	}
	return options[len(options)-1].OptionID
}

func buildPermissionResult(allow bool, optionID string) map[string]any {
	if !allow {
		if optionID == "" {
			return map[string]any{
				"outcome": map[string]any{"outcome": "cancelled"},
			}
		}
		return map[string]any{
			"outcome": map[string]any{
				"outcome":  "selected",
				"optionId": optionID,
			},
		}
	}
	return map[string]any{
		"outcome": map[string]any{
			"outcome":  "selected",
			"optionId": optionID,
		},
	}
}
