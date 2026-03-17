package events

import (
	"strings"
	"sync"
	"time"
)

type Event struct {
	Name      string
	Timestamp time.Time
	Fields    map[string]string
}

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	nextID   int64
	handlers map[string]map[int64]Handler
}

func NewBus() *Bus {
	return &Bus{
		handlers: map[string]map[int64]Handler{},
	}
}

func (b *Bus) Subscribe(eventName string, handler Handler) func() {
	if b == nil || handler == nil {
		return func() {}
	}
	eventName = normalizeEventName(eventName)
	if eventName == "" {
		eventName = "*"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	if b.handlers[eventName] == nil {
		b.handlers[eventName] = map[int64]Handler{}
	}
	b.handlers[eventName][id] = handler
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if rows, ok := b.handlers[eventName]; ok {
			delete(rows, id)
			if len(rows) == 0 {
				delete(b.handlers, eventName)
			}
		}
	}
}

func (b *Bus) Publish(eventName string, fields map[string]string) {
	if b == nil {
		return
	}
	eventName = normalizeEventName(eventName)
	if eventName == "" {
		return
	}
	ev := Event{
		Name:      eventName,
		Timestamp: time.Now().UTC(),
		Fields:    copyFields(fields),
	}

	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.handlers[eventName])+len(b.handlers["*"]))
	for _, h := range b.handlers[eventName] {
		handlers = append(handlers, h)
	}
	for _, h := range b.handlers["*"] {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		h(ev)
	}
}

func normalizeEventName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func copyFields(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
