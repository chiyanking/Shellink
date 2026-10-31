package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"shellink/internal/protocol"
	"shellink/internal/rpc"
)

// Call invokes the shared daemon business API. The stdio transport uses local
// RPC; the daemon's HTTP transport dispatches in-process.
type Call func(method string, params protocol.Value) (protocol.Value, error)

// Server translates MCP tool calls into the shared business API.
type Server struct {
	call Call
}

func New(call Call) *Server {
	return &Server{call: call}
}

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// Serve runs the MCP message loop over newline-delimited JSON-RPC. Diagnostics
// go to log so they never corrupt the protocol stream.
func (s *Server) Serve(in io.Reader, log io.Writer, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req jsonrpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(log, "[mcp] parse error: %v\n", err)
			enc.Encode(&jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonrpcError{Code: -32700, Message: "parse error"},
			})
			continue
		}
		if resp := s.handle(req); resp != nil {
			if err := enc.Encode(resp); err != nil {
				fmt.Fprintf(log, "[mcp] encode error: %v\n", err)
			}
		}
	}
	return scanner.Err()
}

// HandleJSON processes a single raw JSON-RPC request and returns the encoded
// response, or nil for notifications (which expect no response).
func (s *Server) HandleJSON(raw []byte) []byte {
	var req jsonrpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		out, _ := json.Marshal(&jsonrpcResponse{
			JSONRPC: "2.0",
			Error:   &jsonrpcError{Code: -32700, Message: "parse error"},
		})
		return out
	}
	resp := s.handle(req)
	if resp == nil {
		return nil
	}
	out, err := json.Marshal(resp)
	if err != nil {
		out, _ = json.Marshal(&jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &jsonrpcError{Code: -32603, Message: "internal error"},
		})
	}
	return out
}

func (s *Server) handle(req jsonrpcRequest) *jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "shellink", "version": rpc.Version},
			},
		}
	case "notifications/initialized":
		return nil
	case "ping":
		return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return &jsonrpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": Tools}}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		text, err := s.callTool(p.Name, p.Arguments)
		if err != nil {
			return &jsonrpcResponse{
				JSONRPC: "2.0", ID: req.ID,
				Result: map[string]any{
					"content": []map[string]any{{"type": "text", "text": "ERROR: " + err.Error()}},
					"isError": true,
				},
			}
		}
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]any{
				"content": []map[string]any{{"type": "text", "text": text}},
			},
		}
	default:
		return &jsonrpcResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &jsonrpcError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) request(method string, params map[string]any) (string, error) {
	value, err := s.call(method, protocol.JSONValue(params))
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(protocol.ToJSONValue(value))
	return string(data), err
}

func (s *Server) callTool(name string, params map[string]any) (string, error) {
	switch name {
	case "shellink_list_profiles":
		return s.request("profiles.list", nil)
	case "shellink_create_profile":
		return s.request("profiles.create", params)
	case "shellink_list_sessions":
		return s.request("sessions.list", nil)
	case "shellink_create_session":
		args := make(map[string]any, len(params)+1)
		for key, value := range params {
			args[key] = value
		}
		args["source"] = "mcp"
		return s.request("sessions.create", args)
	case "shellink_session_state":
		return s.request("sessions.state", map[string]any{"id": params["id"]})
	case "shellink_read_output":
		limit := params["limit"]
		if limit == nil {
			limit = 2000
		}
		text, err := s.request("sessions.history", map[string]any{
			"id": params["id"], "limit": limit, "includeInternal": true,
		})
		if err != nil {
			return "", err
		}
		return extractPlain([]byte(text)), nil
	case "shellink_send_input":
		return s.request("sessions.input", map[string]any{"id": params["id"], "text": params["text"]})
	case "shellink_close_session":
		return s.request("sessions.close", map[string]any{"id": params["id"]})
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// extractPlain walks a JSON session-history document and concatenates every
// "plain" text field (the human-readable terminal output) into a single string.
func extractPlain(b []byte) string {
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return string(b)
	}
	plain := collectPlain(doc)
	if strings.TrimSpace(plain) == "" {
		return string(b)
	}
	return plain
}

func collectPlain(v any) string {
	switch x := v.(type) {
	case map[string]any:
		var s strings.Builder
		if p, ok := x["plain"].(string); ok {
			s.WriteString(p)
		}
		for _, val := range x {
			s.WriteString(collectPlain(val))
		}
		return s.String()
	case []any:
		var s strings.Builder
		for _, val := range x {
			s.WriteString(collectPlain(val))
		}
		return s.String()
	}
	return ""
}
