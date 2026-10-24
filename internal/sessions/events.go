package sessions

import (
	"sync"

	"shellink/internal/protocol"
)

type Event struct {
	Name string
	Data protocol.Value
}

type Bus struct {
	mu        sync.RWMutex
	listeners map[uint64]func(Event)
	nextID    uint64
}

func NewBus() *Bus { return &Bus{listeners: make(map[uint64]func(Event))} }

func (b *Bus) Subscribe(listener func(Event)) func() {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.listeners[id] = listener
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.listeners, id)
			b.mu.Unlock()
		})
	}
}

func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	listeners := make([]func(Event), 0, len(b.listeners))
	for _, listener := range b.listeners {
		listeners = append(listeners, listener)
	}
	b.mu.RUnlock()
	for _, listener := range listeners {
		listener(event)
	}
}
