package mcpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"shellink/internal/rpc"
)

// Server translates MCP (Model Context Protocol) tool calls into requests
// against the Shellink daemon HTTP API. It is transport agnostic: the same
// implementation backs the stdio CLI and the daemon's HTTP endpoint.
type Server struct {
	base string
	http *http.Client
}

// New returns a Server talking to the given Shellink HTTP API base URL
// (for example http://127.0.0.1:7070/shellink/api).
func New(base string) *Server {
	return &Server{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
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

func (s *Server) api(method, path string, body interface{}) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return data, fmt.Errorf("API %s %s -> %d: %s", method, path, resp.StatusCode, string(data))
	}
	return data, nil
}

func (s *Server) callTool(name string, params map[string]any) (string, error) {
	switch name {
	case "shellink_list_profiles":
		b, err := s.api("GET", "/profiles", nil)
		return string(b), err
	case "shellink_create_profile":
		b, err := s.api("POST", "/profiles", params)
		return string(b), err
	case "shellink_list_sessions":
		b, err := s.api("GET", "/sessions", nil)
		return string(b), err
	case "shellink_create_session":
		b, err := s.api("POST", "/sessions", params)
		return string(b), err
	case "shellink_session_state":
		id, _ := params["id"].(string)
		b, err := s.api("GET", "/sessions/"+id+"/state", nil)
		return string(b), err
	case "shellink_read_output":
		id, _ := params["id"].(string)
		limit := 2000
		if v, ok := params["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		b, err := s.api("GET", fmt.Sprintf("/sessions/%s/history?limit=%d&includeInternal=1", id, limit), nil)
		if err != nil {
			return "", err
		}
		return extractPlain(b), nil
	case "shellink_send_input":
		id, _ := params["id"].(string)
		text, _ := params["text"].(string)
		b, err := s.api("POST", "/sessions/"+id+"/input", map[string]any{"text": text})
		return string(b), err
	case "shellink_close_session":
		id, _ := params["id"].(string)
		b, err := s.api("POST", "/sessions/"+id+"/close", nil)
		return string(b), err
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
