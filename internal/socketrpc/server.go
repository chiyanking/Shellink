package socketrpc

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"shellink/internal/config"
	"shellink/internal/localipc"
	"shellink/internal/protocol"
	"shellink/internal/rpc"
	"shellink/internal/service"
	"shellink/internal/sessions"
)

type Server struct {
	cfg         config.Config
	dispatcher  *rpc.Dispatcher
	bus         *sessions.Bus
	manager     *sessions.Manager
	listener    *localipc.Listener
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	closing     bool
	closeOnce   sync.Once
	wg          sync.WaitGroup
}

func NewServer(cfg config.Config, dispatcher *rpc.Dispatcher, bus *sessions.Bus, manager *sessions.Manager) *Server {
	return &Server{cfg: cfg, dispatcher: dispatcher, bus: bus, manager: manager, connections: make(map[net.Conn]struct{})}
}

func (s *Server) ListenAddress() (string, error) {
	listener, err := localipc.Listen(s.cfg)
	if err != nil {
		return "", err
	}
	s.listener = listener
	return listener.NetListener.Addr().String(), nil
}

func (s *Server) Serve() error {
	if s.listener == nil {
		return errors.New("local RPC listener is not open")
	}
	for {
		conn, err := s.listener.NetListener.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}
		s.connections[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go s.handle(conn)
	}
}

func (s *Server) Close() error {
	var result error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		for conn := range s.connections {
			_ = conn.Close()
		}
		s.mu.Unlock()
		if s.listener != nil {
			result = s.listener.Close()
		}
		s.wg.Wait()
	})
	return result
}

func (s *Server) handle(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.connections, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	decoder := protocol.NewFrameDecoder(s.cfg.MaxFrameBytes)
	sink := &connectionSink{conn: conn, bus: s.bus, manager: s.manager, maxFrame: s.cfg.MaxFrameBytes}
	defer sink.close()
	buffer := make([]byte, 32*1024)
	helloComplete := false
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			frames, decodeErr := decoder.Push(buffer[:n])
			if decodeErr != nil {
				sink.send(protocol.Response, protocol.Object("id", int64(0), "ok", false, "error", rpcError(&service.AppError{Code: "PROTOCOL_ERROR", Message: decodeErr.Error(), Status: 400})))
				return
			}
			for _, frame := range frames {
				if frame.Kind != protocol.Request {
					sink.send(protocol.Response, protocol.Object("id", int64(0), "ok", false, "error", rpcError(&service.AppError{Code: "PROTOCOL_ERROR", Message: "Client may only send request frames", Status: 400})))
					return
				}
				request := protocol.MapValue(frame.Payload)
				id, validID := request["id"]
				method, validMethod := protocol.StringValue(request["method"])
				if !validID || !validRPCID(id) || !validMethod {
					sink.send(protocol.Response, protocol.Object("id", int64(0), "ok", false, "error", rpcError(&service.AppError{Code: "PROTOCOL_ERROR", Message: "Invalid request envelope", Status: 400})))
					return
				}
				params := request["params"]
				if !helloComplete {
					if method != "system.hello" {
						sink.reply(id, nil, &service.AppError{Code: "PROTOCOL_ERROR", Message: "First request must be system.hello", Status: 400})
						return
					}
					if s.listener.Token != "" && protocol.MapValue(params)["socketToken"] != s.listener.Token {
						sink.reply(id, nil, &service.AppError{Code: "UNAVAILABLE", Message: "Local endpoint authentication failed", Status: 401})
						return
					}
					result, dispatchErr := s.dispatcher.Dispatch(method, params, sink)
					sink.reply(id, result, dispatchErr)
					if dispatchErr != nil {
						return
					}
					helloComplete = true
					continue
				}
				go func(id protocol.Value, method string, params protocol.Value) {
					result, dispatchErr := s.dispatcher.Dispatch(method, params, sink)
					sink.reply(id, result, dispatchErr)
				}(id, method, params)
			}
		}
		if err != nil {
			if err == io.EOF {
				_ = decoder.Finish()
			}
			return
		}
	}
}

func validRPCID(value protocol.Value) bool {
	switch value.(type) {
	case string, int64, uint64:
		return true
	default:
		return false
	}
}

type connectionSink struct {
	conn     net.Conn
	bus      *sessions.Bus
	manager  *sessions.Manager
	maxFrame int
	writeMu  sync.Mutex
	mu       sync.Mutex
	subs     map[string]func()
	closed   bool
	hiding   map[string]bool
}

func (s *connectionSink) send(kind protocol.FrameKind, payload protocol.Value) error {
	frame, err := protocol.EncodeFrameLimit(kind, payload, s.maxFrame)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, err = io.Copy(s.conn, bytes.NewReader(frame))
	_ = s.conn.SetWriteDeadline(time.Time{})
	return err
}

func (s *connectionSink) reply(id, result protocol.Value, err error) {
	if err != nil {
		var app *service.AppError
		if !errors.As(err, &app) {
			app = service.Internal(err)
		}
		_ = s.send(protocol.Response, protocol.Object("id", id, "ok", false, "error", rpcError(app)))
		return
	}
	_ = s.send(protocol.Response, protocol.Object("id", id, "ok", true, "result", result))
}

func (s *connectionSink) AddSubscription(sessionID *string, replay bool) (protocol.Value, error) {
	id, err := subscriptionID()
	if err != nil {
		return nil, service.Internal(err)
	}
	initial := protocol.Object("sessions", nil)
	if sessionID != nil {
		state, stateErr := s.manager.State(*sessionID)
		if stateErr != nil {
			return nil, stateErr
		}
		fields := []any{"state", state}
		if replay {
			history, historyErr := s.manager.RawHistory(*sessionID, 512*1024, true)
			if historyErr != nil {
				return nil, historyErr
			}
			fields = append(fields, "replay", history)
		}
		initial = protocol.Object(fields...)
	} else {
		list, listErr := s.manager.List()
		if listErr != nil {
			return nil, listErr
		}
		initial = protocol.Object("sessions", list)
	}
	filteredID := ""
	if sessionID != nil {
		filteredID = *sessionID
	}
	unsubscribe := s.bus.Subscribe(func(event sessions.Event) {
		fields := protocol.MapValue(event.Data)
		eventSessionID, _ := protocol.StringValue(fields["sessionId"])
		if filteredID != "" {
			if eventSessionID != filteredID {
				return
			}
		}
		if event.Name == "session.data" {
			direction, _ := protocol.StringValue(fields["direction"])
			if direction != "output" {
				return
			}
			internal, _ := protocol.BoolValue(fields["internal"])
			if internal {
				s.mu.Lock()
				alreadyHiding := s.hiding[id]
				if s.hiding == nil {
					s.hiding = make(map[string]bool)
				}
				s.hiding[id] = true
				s.mu.Unlock()
				if !alreadyHiding {
					s.sendEvent(id, event.Name, protocol.Object("sessionId", eventSessionID, "direction", "output", "raw", "\r\n[shellink] file transfer in progress...\r\n", "plain", "\n[shellink] file transfer in progress...\n", "internal", false), nil)
				}
				return
			}
			s.mu.Lock()
			wasHiding := s.hiding[id]
			delete(s.hiding, id)
			s.mu.Unlock()
			if wasHiding {
				s.sendEvent(id, event.Name, protocol.Object("sessionId", eventSessionID, "direction", "output", "raw", "\r\n[shellink] transfer output hidden\r\n", "plain", "\n[shellink] transfer output hidden\n", "internal", false), nil)
			}
		}
		var seq *int64
		if value, ok := protocol.IntValue(fields["seq"]); ok {
			seq = &value
		}
		s.sendEvent(id, event.Name, event.Data, seq)
	})
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		unsubscribe()
		return nil, service.Conflict("Connection is closing")
	}
	if s.subs == nil {
		s.subs = make(map[string]func())
	}
	s.subs[id] = unsubscribe
	s.mu.Unlock()
	return protocol.Object("subscriptionId", id, "initial", initial), nil
}

func (s *connectionSink) sendEvent(subscriptionID, name string, data protocol.Value, seq *int64) {
	fields := []any{"subscriptionId", subscriptionID, "event", name, "data", data}
	if seq != nil {
		fields = append(fields, "seq", *seq)
	}
	_ = s.send(protocol.Event, protocol.Object(fields...))
}

func (s *connectionSink) RemoveSubscription(id string) bool {
	s.mu.Lock()
	cleanup, exists := s.subs[id]
	delete(s.subs, id)
	s.mu.Unlock()
	if exists {
		cleanup()
	}
	return exists
}

func (s *connectionSink) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cleanups := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, cleanup := range cleanups {
		cleanup()
	}
}

func rpcError(err *service.AppError) protocol.Value {
	fields := []any{"code", err.Code, "message", err.Message, "status", err.Status}
	if err.Details != nil {
		fields = append(fields, "details", protocol.JSONValue(err.Details))
	}
	return protocol.Object(fields...)
}

func subscriptionID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	value := hex.EncodeToString(bytes[:])
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}
