package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type resourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

type resourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

func (s *Server) listResources() map[string]any {
	return map[string]any{
		"resources": []resourceDefinition{
			{URI: "torque://info", Name: "info", Title: "Torque MCP Info", Description: "Torque MCP server metadata", MIMEType: "application/json"},
			{URI: "torque://sessions", Name: "sessions", Title: "Torque Sessions", Description: "Local MCP sessions", MIMEType: "application/json"},
		},
	}
}

func (s *Server) readResource(ctx context.Context, uri string) (map[string]any, error) {
	uri = strings.TrimSpace(uri)
	switch {
	case uri == "torque://info":
		info := map[string]any{
			"server":       map[string]any{"name": "torque-mcp", "protocolVersion": ProtocolVersion, "version": s.version},
			"remoteAgent":  strings.TrimSpace(s.cfg.RemoteAgent),
			"writeEnabled": s.cfg.EnableWrite,
			"tools":        sortedToolNames(s.tools),
		}
		return contentsJSON(uri, info), nil
	case uri == "torque://sessions":
		return contentsJSON(uri, map[string]any{"sessions": s.sessions.list()}), nil
	case strings.HasPrefix(uri, "torque://sessions/"):
		id := strings.TrimPrefix(uri, "torque://sessions/")
		if idx := strings.Index(id, "/"); idx >= 0 {
			id = id[:idx]
		}
		if rec, ok := s.sessions.get(id); ok {
			return contentsJSON(uri, rec), nil
		}
		if strings.TrimSpace(s.cfg.RemoteAgent) != "" {
			res, err := s.toolSessionGet(ctx, mustRaw(map[string]any{"sessionId": id}))
			if err != nil {
				return nil, err
			}
			return contentsJSON(uri, res.StructuredContent), nil
		}
		return nil, fmt.Errorf("session %q not found", id)
	default:
		return nil, fmt.Errorf("unknown resource URI %q", uri)
	}
}

func contentsJSON(uri string, v any) map[string]any {
	raw, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{
		"contents": []resourceContents{{URI: uri, MIMEType: "application/json", Text: string(raw)}},
	}
}

func mustRaw(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
