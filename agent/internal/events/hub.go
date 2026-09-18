package events

import "sync"

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan string]struct{}
}

func New() *Hub { return &Hub{subscribers: map[chan string]struct{}{}} }
func (h *Hub) Subscribe() (chan string, func()) {
	ch := make(chan string, 32)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() { once.Do(func() { h.mu.Lock(); delete(h.subscribers, ch); close(ch); h.mu.Unlock() }) }
}
func (h *Hub) Publish(value string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- value:
		default:
		}
	}
}
func (h *Hub) Count() int { h.mu.RLock(); defer h.mu.RUnlock(); return len(h.subscribers) }
