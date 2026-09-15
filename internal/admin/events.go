package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// EventBroker manages active Server-Sent Event (SSE) client connections
type EventBroker struct {
	mu      sync.Mutex
	clients map[chan []byte]bool
}

// NewEventBroker initializes an SSE broker
func NewEventBroker() *EventBroker {
	return &EventBroker{
		clients: make(map[chan []byte]bool),
	}
}

// Subscribe adds a new SSE listener
func (b *EventBroker) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 64)
	b.clients[ch] = true
	return ch
}

// Unsubscribe removes an SSE listener
func (b *EventBroker) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.clients[ch]; exists {
		delete(b.clients, ch)
		close(ch)
	}
}

// Broadcast sends a JSON event to all connected listeners
func (b *EventBroker) Broadcast(eventType string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(payload))
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.clients {
		select {
		case ch <- []byte(msg):
		default:
			// Client buffer full; skip to avoid blocking
		}
	}
}

// ServeHTTP handles incoming SSE HTTP streaming connections
func (b *EventBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send initial handshake ping
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
