package client

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"shellink/internal/config"
	"shellink/internal/localipc"
	"shellink/internal/protocol"
	"shellink/internal/service"
)

type response struct {
	result protocol.Value
	err    error
}

type Client struct {
	cfg     config.Config
	conn    net.Conn
	decoder *protocol.FrameDecoder
	writes  sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan response
	nextID  atomic.Uint64
	events  chan protocol.Value
	closed  chan struct{}
	close   sync.Once
}

func Connect(cfg config.Config, timeout time.Duration) (*Client, error) {
	conn, token, err := localipc.Dial(cfg)
	if err != nil {
		return nil, err
	}
	client := &Client{
		cfg: cfg, conn: conn, decoder: protocol.NewFrameDecoder(cfg.MaxFrameBytes),
		pending: make(map[uint64]chan response), events: make(chan protocol.Value, 128), closed: make(chan struct{}),
	}
	go client.readLoop()
	params := []any{"protocolVersion", int64(1), "clientVersion", "0.2.4"}
	if token != "" {
		params = append(params, "socketToken", token)
	}
	if _, err := client.Request("system.hello", protocol.Object(params...), timeout); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Events() <-chan protocol.Value { return c.events }

func (c *Client) Request(method string, params protocol.Value, timeout time.Duration) (protocol.Value, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	id := c.nextID.Add(1)
	result := make(chan response, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return nil, &service.AppError{Code: "UNAVAILABLE", Message: "Daemon is not connected", Status: 503}
	default:
	}
	c.pending[id] = result
	c.mu.Unlock()
	frame, err := protocol.EncodeFrameLimit(protocol.Request,
		protocol.Object("id", id, "method", method, "params", params), c.cfg.MaxFrameBytes)
	if err != nil {
		c.removePending(id)
		return nil, err
	}
	c.writes.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
	_, err = io.Copy(c.conn, bytes.NewReader(frame))
	_ = c.conn.SetWriteDeadline(time.Time{})
	c.writes.Unlock()
	if err != nil {
		c.removePending(id)
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-result:
		return response.result, response.err
	case <-timer.C:
		c.removePending(id)
		return nil, &service.AppError{Code: "TIMEOUT", Message: fmt.Sprintf("Request timed out: %s", method), Status: 504}
	case <-c.closed:
		return nil, &service.AppError{Code: "UNAVAILABLE", Message: "Daemon connection closed", Status: 503}
	}
}

func (c *Client) Close() {
	c.close.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
		c.failPending(&service.AppError{Code: "UNAVAILABLE", Message: "Daemon connection closed", Status: 503})
	})
}

func (c *Client) readLoop() {
	buffer := make([]byte, 32*1024)
	for {
		n, err := c.conn.Read(buffer)
		if n > 0 {
			frames, decodeErr := c.decoder.Push(buffer[:n])
			if decodeErr != nil {
				c.failPending(decodeErr)
				c.Close()
				return
			}
			for _, frame := range frames {
				if frame.Kind == protocol.Event {
					select {
					case c.events <- frame.Payload:
					default:
					}
					continue
				}
				if frame.Kind != protocol.Response {
					continue
				}
				fields := protocol.MapValue(frame.Payload)
				id, ok := protocol.IntValue(fields["id"])
				if !ok || id < 0 {
					continue
				}
				c.mu.Lock()
				pending := c.pending[uint64(id)]
				delete(c.pending, uint64(id))
				c.mu.Unlock()
				if pending == nil {
					continue
				}
				accepted, _ := protocol.BoolValue(fields["ok"])
				if accepted {
					pending <- response{result: fields["result"]}
				} else {
					pending <- response{err: responseError(fields["error"])}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				c.failPending(err)
			}
			c.Close()
			return
		}
	}
}

func (c *Client) removePending(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	values := c.pending
	c.pending = make(map[uint64]chan response)
	c.mu.Unlock()
	for _, pending := range values {
		pending <- response{err: err}
	}
}

func responseError(value protocol.Value) error {
	fields := protocol.MapValue(value)
	code, _ := protocol.StringValue(fields["code"])
	message, _ := protocol.StringValue(fields["message"])
	status, _ := protocol.IntValue(fields["status"])
	if message == "" {
		message = "RPC request failed"
	}
	return &service.AppError{Code: code, Message: message, Status: int(status), Details: protocol.ToJSONValue(fields["details"])}
}
