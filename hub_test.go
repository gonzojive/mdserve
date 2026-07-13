package main

import (
	"testing"
	"time"
)

// TestHub_RegisterAndBroadcast verifies that the event hub correctly registers
// a channel, receives broadcast events, and passes them to the registered channel.
func TestHub_RegisterAndBroadcast(t *testing.T) {
	hub := newHub()
	go hub.run()

	client := make(chan string, 1)
	hub.register <- client

	// Give registration a moment to complete in the event loop
	time.Sleep(50 * time.Millisecond)

	message := "test-reload-event"
	hub.broadcast <- message

	select {
	case msg := <-client:
		if msg != message {
			t.Errorf("Expected message %q, got %q", message, msg)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message broadcast")
	}

	hub.unregister <- client
}
