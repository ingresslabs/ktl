package mcpserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ingresslabs/torque/internal/version"
)

type Server struct {
	cfg      Config
	version  version.Info
	tools    map[string]toolSpec
	sessions *sessionStore
	mu       sync.Mutex
}

func New(cfg Config) *Server {
	s := &Server{
		cfg:      cfg.withDefaults(),
		version:  version.Get(),
		tools:    map[string]toolSpec{},
		sessions: newSessionStore(),
	}
	s.registerTools()
	return s
}

func (s *Server) listTools() map[string]any {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sortStrings(names)
	tools := make([]toolDefinition, 0, len(names))
	for _, name := range names {
		tools = append(tools, s.tools[name].def)
	}
	return map[string]any{"tools": tools}
}

func withTimeout(ctx context.Context, seconds int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		seconds = 300
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

func requireRemote(agent string) error {
	if agent == "" {
		return fmt.Errorf("remote agent is not configured; start torque-mcp with --remote-agent or TORQUE_MCP_REMOTE_AGENT")
	}
	return nil
}

func requireConfirmed(enableWrite bool, confirm bool, action string) error {
	if !enableWrite {
		return fmt.Errorf("%s is disabled; start torque-mcp with --enable-write for mutating tools", action)
	}
	if !confirm {
		return fmt.Errorf("%s requires safety.confirm=true", action)
	}
	return nil
}

func noAdditionalProperties() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false}
}

func objectSchema(props map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func anyObjectSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": true}
}

func stringSchema(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func boolSchema(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func intSchema(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func stringArraySchema(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

func readOnlyAnnotations(title string, openWorld bool) map[string]any {
	return map[string]any{
		"title":           title,
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   openWorld,
	}
}

func writeAnnotations(title string, destructive bool) map[string]any {
	return map[string]any{
		"title":           title,
		"readOnlyHint":    false,
		"destructiveHint": destructive,
		"idempotentHint":  false,
		"openWorldHint":   true,
	}
}
