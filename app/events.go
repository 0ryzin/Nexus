package main

import (
	"encoding/json"
	"sync"
)

// Event is pushed to the UI over Server-Sent Events.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type EventBus struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func NewEventBus() *EventBus { return &EventBus{subs: map[chan []byte]struct{}{}} }

func (b *EventBus) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *EventBus) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

func (b *EventBus) Emit(typ string, data any) {
	msg, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // slow consumer: drop rather than block the emulator monitor
		}
	}
}

// UIHost is implemented by the native window (Windows) or a no-op (dev).
type UIHost interface {
	SetFullscreen(on bool)
	IsFullscreen() bool
	Minimize()
	Restore()
	Quit()
	PickFolder(title string) (string, error)
	PickFile(title, filter string) (string, error)
	Native() bool
	WindowHandle() uintptr
}

type noopUI struct{}

func (noopUI) SetFullscreen(bool)                      {}
func (noopUI) IsFullscreen() bool                      { return false }
func (noopUI) Minimize()                               {}
func (noopUI) Restore()                                {}
func (noopUI) Quit()                                   {}
func (noopUI) PickFolder(string) (string, error)       { return "", errNotSupported }
func (noopUI) PickFile(string, string) (string, error) { return "", errNotSupported }
func (noopUI) Native() bool                            { return false }
func (noopUI) WindowHandle() uintptr                   { return 0 }
