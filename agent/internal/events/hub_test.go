package events

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	h := New()
	ch, cancel := h.Subscribe()
	defer cancel()
	h.Publish("event")
	select {
	case got := <-ch:
		if got != "event" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("event not delivered")
	}
}
