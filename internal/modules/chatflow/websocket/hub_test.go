package websocket

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHubIntegration(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	// Start Hub in background
	go hub.Run()

	// 1. Simulate a Client
	mockClient := &Client{
		hub:    hub,
		send:   make(chan []byte, 256),
		logger: logger,
	}

	// 2. Register Client
	hub.register <- mockClient

	// Give a moment for registration
	time.Sleep(50 * time.Millisecond)

	// 3. Broadcast Message
	testMsg := []byte("test_payload")
	hub.Broadcast <- testMsg

	// 4. Verify Receipt
	select {
	case received := <-mockClient.send:
		if string(received) != string(testMsg) {
			t.Errorf("Expected %s, got %s", testMsg, received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout: Client did not receive broadcast")
	}

	// 5. Unregister Client
	hub.unregister <- mockClient
	time.Sleep(50 * time.Millisecond)

}

func TestHub_BroadcastJSON(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)

	// Start Hub in background (prevents blocking the Broadcast channel)
	go hub.Run()

	mockClient := &Client{
		hub:    hub,
		send:   make(chan []byte, 256),
		logger: logger,
	}

	hub.register <- mockClient
	time.Sleep(50 * time.Millisecond)

	type TestPayload struct {
		Message string `json:"message"`
		Value   int    `json:"value"`
	}

	payload := TestPayload{
		Message: "hello",
		Value:   42,
	}

	err := hub.BroadcastJSON(payload)
	if err != nil {
		t.Fatalf("BroadcastJSON failed: %v", err)
	}

	select {
	case received := <-mockClient.send:
		expected := `{"message":"hello","value":42}`
		if string(received) != expected {
			t.Errorf("Expected %s, got %s", expected, string(received))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout: Client did not receive broadcast")
	}

	hub.unregister <- mockClient
}

func TestHub_BroadcastJSON_Success(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	// Using a buffered channel allows us to test BroadcastJSON without a running Hub.Run() goroutine.
	hub.Broadcast = make(chan []byte, 1)

	payload := map[string]string{"foo": "bar"}
	err := hub.BroadcastJSON(payload)
	if err != nil {
		t.Fatalf("BroadcastJSON failed: %v", err)
	}

	select {
	case data := <-hub.Broadcast:
		expected := `{"foo":"bar"}`
		if string(data) != expected {
			t.Errorf("Expected %s, got %s", expected, string(data))
		}
	default:
		t.Error("Message was not sent to Broadcast channel")
	}
}

func TestHub_BroadcastJSON_MarshalError(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	hub.Broadcast = make(chan []byte, 1)

	// A channel cannot be marshaled to JSON, which will trigger an error.
	payload := make(chan int)
	err := hub.BroadcastJSON(payload)
	if err == nil {
		t.Error("Expected error when marshaling non-serializable payload, got nil")
	}
}
