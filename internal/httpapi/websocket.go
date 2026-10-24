package httpapi

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"shellink/internal/protocol"
	"shellink/internal/sessions"
)

type wsConn struct {
	conn net.Conn
	read *bufio.Reader
	mu   sync.Mutex
}

func (s *Server) terminalWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.websocketAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	sessionID := r.PathValue("id")
	replay, _ := s.sessions.RawHistory(sessionID, 512*1024, false)
	_ = conn.send(map[string]any{"type": "replay", "data": replay})
	state := map[string]any{"state": string(sessions.Disconnected), "mode": string(sessions.Auto)}
	if value, err := s.sessions.State(sessionID); err == nil {
		for key, field := range protocol.MapValue(value) {
			state[key] = protocol.ToJSONValue(field)
		}
	}
	_ = conn.send(map[string]any{"type": "status", "state": state["state"], "mode": state["mode"]})

	var eventMu sync.Mutex
	hidingInternal := false
	unsubscribe := s.bus.Subscribe(func(event sessions.Event) {
		fields := protocol.MapValue(event.Data)
		id, _ := protocol.StringValue(fields["sessionId"])
		if id != sessionID {
			return
		}
		switch event.Name {
		case "session.data":
			direction, _ := protocol.StringValue(fields["direction"])
			if direction != "output" {
				return
			}
			internal, _ := protocol.BoolValue(fields["internal"])
			if internal {
				eventMu.Lock()
				wasHidden := hidingInternal
				hidingInternal = true
				eventMu.Unlock()
				if !wasHidden {
					_ = conn.send(map[string]any{"type": "data", "data": "\r\n[shellink] file transfer in progress...\r\n"})
				}
				return
			}
			eventMu.Lock()
			wasHidden := hidingInternal
			hidingInternal = false
			eventMu.Unlock()
			if wasHidden {
				_ = conn.send(map[string]any{"type": "data", "data": "\r\n[shellink] transfer output hidden\r\n"})
			}
			raw, _ := protocol.StringValue(fields["raw"])
			_ = conn.send(map[string]any{"type": "data", "data": raw})
		case "session.state":
			value, _ := protocol.StringValue(fields["state"])
			_ = conn.send(map[string]any{"type": "state", "state": value})
		case "session.mode":
			value, _ := protocol.StringValue(fields["mode"])
			_ = conn.send(map[string]any{"type": "mode", "mode": value})
		case "session.closed":
			reason, _ := protocol.StringValue(fields["reason"])
			_ = conn.send(map[string]any{"type": "closed", "reason": reason})
		}
	})
	defer unsubscribe()
	for {
		message, err := conn.readMessage()
		if err != nil {
			return
		}
		var input struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if json.Unmarshal(message, &input) != nil {
			continue
		}
		switch input.Type {
		case "input":
			_ = s.sessions.TerminalInput(sessionID, input.Data)
		case "resize":
			if input.Cols > 0 && input.Rows > 0 {
				_ = s.sessions.Resize(sessionID, input.Cols, input.Rows)
			}
		}
	}
}

func (s *Server) eventsWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.websocketAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgradeWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()
	unsubscribe := s.bus.Subscribe(func(event sessions.Event) {
		kind := map[string]string{
			"session.created": "created", "session.state": "state", "session.mode": "mode",
			"session.closed": "closed", "session.loginExternal": "loginExternal", "webhook.received": "webhookReceived",
		}[event.Name]
		if kind == "" {
			return
		}
		payload := make(map[string]any)
		for key, value := range protocol.MapValue(event.Data) {
			payload[key] = protocol.ToJSONValue(value)
		}
		payload["type"] = kind
		_ = conn.send(payload)
	})
	defer unsubscribe()
	for {
		if _, err := conn.readMessage(); err != nil {
			return
		}
	}
}

func (s *Server) websocketAuthorized(r *http.Request) bool {
	return s.localRequest(r) || subtleTokenEqual(r.URL.Query().Get("token"), s.cfg.Token)
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		http.Error(w, "WebSocket upgrade required", http.StatusBadRequest)
		return nil, errors.New("missing WebSocket upgrade headers")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing WebSocket key", http.StatusBadRequest)
		return nil, errors.New("missing WebSocket key")
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket is unavailable", http.StatusInternalServerError)
		return nil, errors.New("response writer cannot hijack connection")
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}
	hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept := base64.StdEncoding.EncodeToString(hash[:])
	_, err = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err == nil {
		err = buffered.Flush()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, read: buffered.Reader}, nil
}

func (c *wsConn) send(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.writeFrame(1, data)
}

func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := make([]byte, 0, len(payload)+10)
	frame = append(frame, 0x80|opcode)
	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 0xffff:
		frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		frame = append(frame, 127, 0, 0, 0, 0, byte(uint64(len(payload))>>24), byte(uint64(len(payload))>>16), byte(uint64(len(payload))>>8), byte(len(payload)))
	}
	frame = append(frame, payload...)
	_, err := c.conn.Write(frame)
	return err
}

func (c *wsConn) readMessage() ([]byte, error) {
	for {
		var header [2]byte
		if _, err := io.ReadFull(c.read, header[:]); err != nil {
			return nil, err
		}
		final := header[0]&0x80 != 0
		opcode := header[0] & 0x0f
		masked := header[1]&0x80 != 0
		length := uint64(header[1] & 0x7f)
		if length == 126 {
			var ext [2]byte
			if _, err := io.ReadFull(c.read, ext[:]); err != nil {
				return nil, err
			}
			length = uint64(binary.BigEndian.Uint16(ext[:]))
		} else if length == 127 {
			var ext [8]byte
			if _, err := io.ReadFull(c.read, ext[:]); err != nil {
				return nil, err
			}
			length = binary.BigEndian.Uint64(ext[:])
		}
		if !masked || !final || length > 1<<20 {
			return nil, errors.New("invalid or oversized WebSocket frame")
		}
		var mask [4]byte
		if _, err := io.ReadFull(c.read, mask[:]); err != nil {
			return nil, err
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(c.read, payload); err != nil {
			return nil, err
		}
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
		switch opcode {
		case 1, 2:
			return payload, nil
		case 8:
			return nil, io.EOF
		case 9:
			if err := c.writeFrame(10, payload); err != nil {
				return nil, err
			}
		case 10:
			continue
		default:
			return nil, errors.New("unsupported WebSocket opcode")
		}
	}
}

func (c *wsConn) Close() error { return c.conn.Close() }
