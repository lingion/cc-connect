//go:build blackbox

package platform_sim

// Blank-import all agent packages so their init() calls register them in
// core.Registry before any test in this package runs.
import (
	_ "github.com/chenhg5/cc-connect/agent/claudecode"
	_ "github.com/chenhg5/cc-connect/agent/codex"
	_ "github.com/chenhg5/cc-connect/agent/cursor"
	_ "github.com/chenhg5/cc-connect/agent/gemini"
	_ "github.com/chenhg5/cc-connect/agent/opencode"
)
