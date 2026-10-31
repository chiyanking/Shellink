// Package desktopbridge adapts the existing local RPC client to a private
// newline-delimited JSON pipe owned by Tauri. Closing it never stops daemon.
package desktopbridge

import (
	"bufio"
	"encoding/json"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"shellink/internal/config"
	"shellink/internal/daemon"
	"shellink/internal/protocol"
	"shellink/internal/service"
)

func Run(cfg config.Config, in io.Reader, out io.Writer) error {
	conn, err := daemon.Ensure(cfg, 15*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.Request("desktop.info", protocol.Object(), 5*time.Second); err != nil {
		return fmt.Errorf("当前后台版本不支持桌面桥接；桌面端没有停止后台或其中的会话。请先保存工作，待现有会话结束后再正常停止旧后台，并用新版核心启动: %w", err)
	}
	var writes sync.Mutex
	encode := func(value any) error { writes.Lock(); defer writes.Unlock(); return json.NewEncoder(out).Encode(value) }
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case event := <-conn.Events():
				if err := encode(map[string]any{"event": protocol.ToJSONValue(event)}); err != nil {
					return
				}
			case <-stop:
				return
			}
		}
	}()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	var requests sync.WaitGroup
	defer requests.Wait()
	for scanner.Scan() {
		var request struct {
			ID        uint64 `json:"id"`
			Method    string `json:"method"`
			Params    any    `json:"params"`
			TimeoutMS int    `json:"timeoutMs"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return err
		}
		requests.Add(1)
		go func() {
			defer requests.Done()
			timeout := time.Duration(request.TimeoutMS) * time.Millisecond
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			if timeout > 3*time.Minute {
				timeout = 3 * time.Minute
			}
			params := protocol.JSONValue(request.Params)
			var err error
			if request.Method == "sessions.upload" || request.Method == "sessions.uploadStart" {
				fields := protocol.MapValue(params)
				encoded, ok := protocol.StringValue(fields["data"])
				if !ok { err = service.Invalid("data must be base64") } else {
					var data []byte
					data, err = base64.StdEncoding.DecodeString(encoded)
					if err != nil { err = service.Invalid("data must be base64") } else {
						for i := range params.([]protocol.KV) { if params.([]protocol.KV)[i].Key == "data" { params.([]protocol.KV)[i].Val = data } }
					}
				}
			}
			var result protocol.Value
			if err == nil { result, err = conn.Request(request.Method, params, timeout) }
			response := map[string]any{"id": request.ID, "result": protocol.ToJSONValue(result)}
			if err != nil {
				var app *service.AppError
				if !errors.As(err, &app) {
					app = service.Internal(err)
				}
				response["error"] = app
				delete(response, "result")
			}
			_ = encode(response)
		}()
	}
	return scanner.Err()
}
