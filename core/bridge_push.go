package core

import (
	"log/slog"
)

// ---------------------------------------------------------------------------
// BridgeServer broadcast helpers
// ---------------------------------------------------------------------------

// broadcastToControlPlane sends a message to all connected adapters that
// requested the "capabilities_snapshot_v1" control-plane protocol.
func (bs *BridgeServer) broadcastToControlPlane(msg map[string]any) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	for _, a := range bs.adapters {
		if !bridgeMetadataStringListContains(a.metadata, "control_plane", bridgeCapabilitiesSnapshotProto) {
			continue
		}
		if err := writeJSON(a.conn, &a.writeMu, msg); err != nil {
			slog.Debug("bridge: broadcast failed", "platform", a.platform, "error", err)
		}
	}
}

// ---------------------------------------------------------------------------
// session_list_update — push when sessions change for a specific session key
// ---------------------------------------------------------------------------

// BroadcastSessionListForKey pushes the session list for a given session key.
func (bs *BridgeServer) BroadcastSessionListForKey(projectName, sessionKey string) {
	bs.enginesMu.RLock()
	ref, ok := bs.engines[projectName]
	bs.enginesMu.RUnlock()
	if !ok || ref.engine == nil {
		return
	}

	sessions := ref.engine.sessions.ListSessions(sessionKey)
	activeID := ref.engine.sessions.ActiveSessionID(sessionKey)

	list := make([]map[string]any, len(sessions))
	for i, s := range sessions {
		list[i] = map[string]any{
			"id":            s.ID,
			"name":          s.GetName(),
			"history_count": len(s.History),
		}
	}

	bs.broadcastToControlPlane(map[string]any{
		"type":        "session_list_update",
		"project":     projectName,
		"session_key": sessionKey,
		"sessions":    list,
		"active_id":   activeID,
	})
}

// ---------------------------------------------------------------------------
// agent_status_update — push when agent status changes
// ---------------------------------------------------------------------------

// BroadcastAgentStatusUpdate pushes the agent status for a project
// to all control-plane adapters.
func (bs *BridgeServer) BroadcastAgentStatusUpdate(projectName string) {
	bs.enginesMu.RLock()
	ref, ok := bs.engines[projectName]
	bs.enginesMu.RUnlock()
	if !ok || ref.engine == nil {
		return
	}

	status := ref.engine.bridgeAgentStatus()
	agentType := ref.engine.AgentTypeName()

	bs.broadcastToControlPlane(map[string]any{
		"type":       "agent_status_update",
		"project":    projectName,
		"status":     status,
		"agent_type": agentType,
	})
}

// ---------------------------------------------------------------------------
// history_sync — push session history on demand
// ---------------------------------------------------------------------------

type bridgeFetchHistory struct {
	Type       string `json:"type"`
	SessionKey string `json:"session_key"`
	SessionID  string `json:"session_id"`
	Project    string `json:"project,omitempty"`
	BeforeTS   int64  `json:"before_timestamp,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// handleFetchHistory processes a "fetch_history" request from a client.
func (bs *BridgeServer) handleFetchHistory(a *bridgeAdapter, req *bridgeFetchHistory) {
	ref := bs.resolveEngine(req.SessionKey, req.Project)
	if ref == nil {
		return
	}

	s := ref.engine.sessions.FindByID(req.SessionID)
	if s == nil {
		return
	}

	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	all := s.GetHistory(0)

	var filtered []HistoryEntry
	for _, h := range all {
		ts := h.Timestamp.Unix()
		if req.BeforeTS > 0 && ts >= req.BeforeTS {
			continue
		}
		filtered = append(filtered, h)
	}

	hasOlder := len(filtered) > limit
	if hasOlder {
		filtered = filtered[len(filtered)-limit:]
	}

	entries := make([]map[string]any, len(filtered))
	for i, h := range filtered {
		entries[i] = map[string]any{
			"role":      h.Role,
			"content":   h.Content,
			"timestamp": h.Timestamp.Unix(),
		}
	}

	_ = writeJSON(a.conn, &a.writeMu, map[string]any{
		"type":        "history_sync",
		"project":     req.Project,
		"session_key": req.SessionKey,
		"session_id":  req.SessionID,
		"entries":     entries,
		"has_older":   hasOlder,
	})
}

// pushInitialHistorySync sends the active session's recent history after
// registration for control-plane adapters.
func (bs *BridgeServer) pushInitialHistorySync(a *bridgeAdapter) {
	bs.enginesMu.RLock()
	defer bs.enginesMu.RUnlock()

	for projectName, ref := range bs.engines {
		if ref == nil || ref.engine == nil {
			continue
		}
		bs.pushHistorySyncForProjects(a, projectName, ref)
	}
}

func (bs *BridgeServer) pushHistorySyncForProjects(a *bridgeAdapter, projectName string, ref *bridgeEngineRef) {
	sessions := ref.engine.sessions
	if sessions == nil {
		return
	}

	// We don't know the adapter's session key yet, so we broadcast for
	// all known session keys that have an active session with history.
	sessions.ForEachKey(func(sessionKey string) {
		activeID := sessions.ActiveSessionID(sessionKey)
		if activeID == "" {
			return
		}
		s := sessions.FindByID(activeID)
		if s == nil {
			return
		}
		hist := s.GetHistory(50)
		if len(hist) == 0 {
			return
		}
		entries := make([]map[string]any, len(hist))
		for i, h := range hist {
			entries[i] = map[string]any{
				"role":      h.Role,
				"content":   h.Content,
				"timestamp": h.Timestamp.Unix(),
			}
		}
		_ = writeJSON(a.conn, &a.writeMu, map[string]any{
			"type":        "history_sync",
			"project":     projectName,
			"session_key": sessionKey,
			"session_id":  activeID,
			"entries":     entries,
			"has_older":   len(hist) >= 50,
		})
	})
}

// ---------------------------------------------------------------------------
// Engine hooks — call these from engine state transitions
// ---------------------------------------------------------------------------

// NotifyBridgeSessionChange should be called when sessions are created,
// deleted, or switched for a project.
func (e *Engine) NotifyBridgeSessionChange(sessionKey string) {
	for _, p := range e.platforms {
		if bp, ok := p.(*BridgePlatform); ok {
			bp.server.BroadcastSessionListForKey(e.name, sessionKey)
			return
		}
	}
}

// NotifyBridgeAgentStatus should be called when agent status changes
// (session start/stop, permission request/response).
func (e *Engine) NotifyBridgeAgentStatus() {
	for _, p := range e.platforms {
		if bp, ok := p.(*BridgePlatform); ok {
			bp.server.BroadcastAgentStatusUpdate(e.name)
			return
		}
	}
}
