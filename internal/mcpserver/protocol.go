package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type toolDefinition struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	Annotations  map[string]any `json:"annotations,omitempty"`
	Execution    map[string]any `json:"execution,omitempty"`
}

type contentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	URI      string `json:"uri,omitempty"`
	Name     string `json:"name,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
}

type toolResult struct {
	Content           []contentItem `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type toolExecutionError struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Retryable bool     `json:"retryable"`
	Hints     []string `json:"hints,omitempty"`
	Details   any      `json:"details,omitempty"`
}

type toolHandler func(context.Context, json.RawMessage) (toolResult, error)

type toolSpec struct {
	def     toolDefinition
	handler toolHandler
}

func (s *Server) handleMessage(ctx context.Context, data []byte) ([]byte, bool) {
	var req rpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error", Data: err.Error()},
		}), true
	}
	if req.JSONRPC != "2.0" {
		return s.protocolError(req.ID, -32600, "invalid request", "jsonrpc must be 2.0"), req.ID != nil
	}
	if req.Method == "" {
		return s.protocolError(req.ID, -32600, "invalid request", "method is required"), req.ID != nil
	}
	if req.ID == nil {
		s.handleNotification(ctx, req)
		return nil, false
	}
	result, errResp := s.handleRequest(ctx, req)
	if errResp != nil {
		return mustMarshal(rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: errResp}), true
	}
	return mustMarshal(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}), true
}

func (s *Server) handleNotification(_ context.Context, _ rpcRequest) {
}

func (s *Server) handleRequest(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": true, "listChanged": true},
				"prompts":   map[string]any{"listChanged": false},
				"logging":   map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":        "torque-mcp",
				"title":       "Torque MCP Server",
				"version":     s.version.Version,
				"description": "Agent-facing MCP adapter for Torque delivery workflows",
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "logging/setLevel":
		return map[string]any{}, nil
	case "tools/list":
		return s.listTools(), nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(defaultObject(req.Params), &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid tools/call params", Data: err.Error()}
		}
		spec, ok := s.tools[params.Name]
		if !ok {
			return nil, &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}
		}
		res, err := spec.handler(ctx, defaultObject(params.Arguments))
		if err != nil {
			return textToolError("TOOL_EXECUTION_ERROR", err.Error(), true, nil, nil), nil
		}
		return res, nil
	case "resources/list":
		return s.listResources(), nil
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(defaultObject(req.Params), &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid resources/read params", Data: err.Error()}
		}
		res, err := s.readResource(ctx, params.URI)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		return res, nil
	case "prompts/list":
		return s.listPrompts(), nil
	case "prompts/get":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(defaultObject(req.Params), &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid prompts/get params", Data: err.Error()}
		}
		prompt, err := s.getPrompt(params.Name, params.Arguments)
		if err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		return prompt, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) protocolError(id *json.RawMessage, code int, message string, data any) []byte {
	return mustMarshal(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}})
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func defaultObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func decodeArgs(raw json.RawMessage, v any) error {
	if v == nil {
		return errors.New("decode target is nil")
	}
	if err := json.Unmarshal(defaultObject(raw), v); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	return nil
}

func textResult(text string, structured any, links ...contentItem) toolResult {
	content := []contentItem{{Type: "text", Text: text}}
	content = append(content, links...)
	return toolResult{Content: content, StructuredContent: structured}
}

func textToolError(code, message string, retryable bool, hints []string, details any) toolResult {
	errObj := toolExecutionError{Code: code, Message: message, Retryable: retryable, Hints: hints, Details: details}
	data, _ := json.Marshal(errObj)
	return toolResult{
		Content:           []contentItem{{Type: "text", Text: string(data)}},
		StructuredContent: errObj,
		IsError:           true,
	}
}

func resourceLink(uri, name, desc, mime string) contentItem {
	return contentItem{Type: "resource_link", URI: uri, Name: name, Text: desc, MIMEType: mime}
}
