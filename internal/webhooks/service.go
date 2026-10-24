package webhooks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"shellink/internal/protocol"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/store"
)

type Message struct {
	ID         string `json:"id"`
	ReceivedAt int64  `json:"receivedAt"`
	Data       any    `json:"data"`
}

type Service struct {
	db       *store.Store
	bus      *sessions.Bus
	mu       sync.Mutex
	messages []Message
}

func New(db *store.Store, bus *sessions.Bus) *Service {
	s := &Service{db: db, bus: bus}
	bus.Subscribe(s.forward)
	return s
}

func (s *Service) List() (protocol.Value, error) {
	rows, err := s.db.Webhooks()
	if err != nil {
		return nil, service.Internal(err)
	}
	values := make([]protocol.Value, 0, len(rows))
	for _, row := range rows {
		values = append(values, webhookValue(row))
	}
	return values, nil
}

func (s *Service) Create(input protocol.Value) (protocol.Value, error) {
	fields := protocol.MapValue(input)
	urlText, ok := protocol.StringValue(fields["url"])
	parsed, parseErr := url.ParseRequestURI(urlText)
	if !ok || parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, service.Invalid("url must be a valid HTTP or HTTPS URL")
	}
	events := []string{}
	if raw, exists := fields["events"]; exists && raw != nil {
		values, valid := raw.([]protocol.Value)
		if !valid {
			return nil, service.Invalid("events must be an array")
		}
		for _, item := range values {
			name, valid := protocol.StringValue(item)
			if !valid || (name != "state" && name != "closed" && name != "loginExternal") {
				return nil, service.Invalid("events must contain state, closed, or loginExternal")
			}
			events = append(events, name)
		}
	}
	row := store.Webhook{ID: uuid(), URL: urlText, Events: events, CreatedAt: time.Now().UnixMilli()}
	if err := s.db.InsertWebhook(row); err != nil {
		return nil, service.Internal(err)
	}
	return webhookValue(row), nil
}

func (s *Service) Delete(id string) error {
	if err := s.db.DeleteWebhook(id); err != nil {
		return service.Internal(err)
	}
	return nil
}

func (s *Service) Receive(data any) Message {
	message := Message{ID: uuid(), ReceivedAt: time.Now().UnixMilli(), Data: data}
	s.mu.Lock()
	s.messages = append(s.messages, message)
	if len(s.messages) > 200 {
		s.messages = append([]Message(nil), s.messages[len(s.messages)-200:]...)
	}
	s.mu.Unlock()
	s.bus.Publish(sessions.Event{Name: "webhook.received", Data: protocol.JSONValue(map[string]any{
		"id": message.ID, "receivedAt": message.ReceivedAt, "data": data,
	})})
	return message
}

func (s *Service) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Message, len(s.messages))
	for i := range s.messages {
		result[len(result)-1-i] = s.messages[i]
	}
	return result
}

func (s *Service) ClearMessages() {
	s.mu.Lock()
	s.messages = nil
	s.mu.Unlock()
}

func (s *Service) forward(event sessions.Event) {
	name := ""
	switch event.Name {
	case "session.state":
		name = "state"
	case "session.closed":
		name = "closed"
	case "session.loginExternal":
		name = "loginExternal"
	default:
		return
	}
	rows, err := s.db.Webhooks()
	if err != nil {
		return
	}
	for _, row := range rows {
		if !contains(row.Events, name) && len(row.Events) != 0 {
			continue
		}
		payload := make(map[string]any)
		for key, value := range protocol.MapValue(event.Data) {
			payload[key] = protocol.ToJSONValue(value)
		}
		payload["event"] = name
		payload["at"] = time.Now().UnixMilli()
		go deliver(row.URL, payload)
	}
}

func webhookValue(row store.Webhook) protocol.Value {
	events := make([]protocol.Value, len(row.Events))
	for i, event := range row.Events {
		events[i] = event
	}
	return protocol.Object("id", row.ID, "url", row.URL, "events", events, "createdAt", row.CreatedAt)
}

func deliver(target string, payload map[string]any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Printf("[webhook] delivery failed for %s: %v\n", target, err)
		return
	}
	response.Body.Close()
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uuid() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	value := hex.EncodeToString(bytes[:])
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}
