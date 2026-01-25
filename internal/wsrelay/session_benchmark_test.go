package wsrelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
)

func TestSession_Send_Correctness(t *testing.T) {
	// Setup echo server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				break
			}
			// Echo back
			err = c.WriteMessage(mt, message)
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	// Connect client (which will be our session's conn)
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Create session
	sess := newSession(conn, nil, "test-session")

	// Test data
	payload := map[string]any{
		"foo": "bar",
		"nested": map[string]any{
			"a": 1.0, // decoding into interface{} often gives float64
			"b": true,
		},
	}
	msg := Message{
		ID:      "msg-123",
		Type:    MessageTypeHTTPReq,
		Payload: payload,
	}

	// Read from server side (echoed back)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, p, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("ReadMessage failed: %v", err)
			return
		}

		var received Message
		if err := json.Unmarshal(p, &received); err != nil {
			t.Errorf("Unmarshal failed: %v", err)
			return
		}

		if received.ID != msg.ID {
			t.Errorf("Expected ID %s, got %s", msg.ID, received.ID)
		}
		if received.Type != msg.Type {
			t.Errorf("Expected Type %s, got %s", msg.Type, received.Type)
		}
		// Basic deep checks could be added here, but ID/Type match confirms structure
	}()

	// Send via session (function under test)
	if err := sess.send(context.Background(), msg); err != nil {
		t.Fatalf("session.send failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for echo")
	}
}

func BenchmarkSession_Send(b *testing.B) {
	// Setup discard server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	sess := newSession(conn, nil, "bench-session")
	msg := Message{
		ID:   "bench-msg",
		Type: "bench",
		Payload: map[string]any{
			"data": strings.Repeat("X", 1024), // 1KB payload
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sess.send(context.Background(), msg); err != nil {
			b.Fatalf("send failed: %v", err)
		}
	}
}
