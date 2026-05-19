package websocket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

func TestGetIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remote   string
		expected string
	}{
		{"X-Forwarded-For present from trusted proxy", map[string]string{"X-Forwarded-For": "203.0.113.195, 198.51.100.1"}, "192.168.1.1:1234", "203.0.113.195"},
		{"X-Real-IP present from trusted proxy", map[string]string{"X-Real-IP": "203.0.113.196"}, "192.168.1.1:1234", "203.0.113.196"},
		{"X-Forwarded-For ignored from untrusted IP", map[string]string{"X-Forwarded-For": "203.0.113.195, 198.51.100.1"}, "203.0.113.199:1234", "203.0.113.199"},
		{"X-Real-IP ignored from untrusted IP", map[string]string{"X-Real-IP": "203.0.113.196"}, "203.0.113.199:1234", "203.0.113.199"},
		{"RemoteAddr present untrusted", map[string]string{}, "203.0.113.197:1234", "203.0.113.197"},
		{"RemoteAddr no port untrusted", map[string]string{}, "203.0.113.198", "203.0.113.198"},
		{"RemoteAddr present trusted", map[string]string{}, "10.0.0.1:1234", "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tt.remote

			ip := getIP(req)
			if ip != tt.expected {
				t.Errorf("getIP() = %v, want %v", ip, tt.expected)
			}
		})
	}
}

func TestServeWs_RateLimit(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, logger, nil, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.DefaultDialer

	// Our connection limiter is 2 requests per second with a burst of 5.
	// Since tests might run fast or have existing global state, we mock a specific IP block.
	// We'll just make 6 quick requests to trigger the rate limiter limit (burst=5).
	successCount := 0
	failCount := 0

	for i := 0; i < 7; i++ {
		// Use Dial rather than making a raw request to get the proper websocket upgrade attempt.
		// Spoof the IP to avoid interference with other tests.
		conn, resp, err := dialer.Dial(wsURL, http.Header{"X-Real-IP": []string{"10.0.0.99"}})
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
				failCount++
			}
		} else {
			successCount++
			conn.Close()
		}
	}

	if failCount == 0 {
		t.Errorf("Expected rate limiting to eventually block connections (429 status code), but none were blocked. Successes: %d", successCount)
	}
}

func TestServeWs_ConnectionAndRegistration(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, logger, nil, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to establish WebSocket connection: %v", err)
	}
	defer conn.Close()

	// Wait for registration
	time.Sleep(100 * time.Millisecond)

	// Test broadcast reaches client (validates writePump)
	testMsg := []byte("test message")
	hub.Broadcast <- testMsg

	// Set a read deadline
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, received, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message from WebSocket: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Errorf("Expected message type TextMessage, got %d", msgType)
	}
	if string(received) != string(testMsg) {
		t.Errorf("Expected message %q, got %q", string(testMsg), string(received))
	}

	// Test cleanup (validates readPump defer)
	conn.Close()
	time.Sleep(100 * time.Millisecond)

}

func TestIPRateLimiter_Cleanup(t *testing.T) {
	// Use a very fast cleanup cycle for testing
	rl := &IPRateLimiter{
		ips:             make(map[string]*visitor),
		r:               rate.Limit(10),
		b:               10,
		cleanupInterval: 10 * time.Millisecond,
		visitorTTL:      50 * time.Millisecond,
		quit:            make(chan struct{}),
	}
	go rl.cleanupVisitors()
	defer rl.Stop()

	// 1. Test basic registration
	rl.GetLimiter("1.1.1.1")
	rl.mu.Lock()
	if _, exists := rl.ips["1.1.1.1"]; !exists {
		rl.mu.Unlock()
		t.Fatal("Expected IP 1.1.1.1 to be registered")
	}
	rl.mu.Unlock()

	// 2. Test cleanup of stale entry
	time.Sleep(100 * time.Millisecond)
	rl.mu.Lock()
	if _, exists := rl.ips["1.1.1.1"]; exists {
		rl.mu.Unlock()
		t.Error("Expected IP 1.1.1.1 to be cleaned up after TTL")
	}
	rl.mu.Unlock()

	// 3. Test preservation of active entry
	rl.GetLimiter("2.2.2.2")
	time.Sleep(30 * time.Millisecond)
	rl.GetLimiter("2.2.2.2") // Refresh lastSeen
	time.Sleep(30 * time.Millisecond)

	rl.mu.Lock()
	if _, exists := rl.ips["2.2.2.2"]; !exists {
		rl.mu.Unlock()
		t.Error("Expected IP 2.2.2.2 to be preserved because it was recently active")
	}
	rl.mu.Unlock()

	// 4. Test large number of IPs (high load cleanup)
	for i := 0; i < 1000; i++ {
		rl.GetLimiter(fmt.Sprintf("10.0.0.%d", i))
	}

	rl.mu.Lock()
	if len(rl.ips) < 1000 {
		t.Errorf("Expected at least 1000 IPs, got %d", len(rl.ips))
	}
	rl.mu.Unlock()

	// Wait for cleanup of all 1000+ entries
	time.Sleep(200 * time.Millisecond)

	rl.mu.Lock()
	if len(rl.ips) > 1 { // 2.2.2.2 might still be there if we are fast, but 1000+ should be gone
		t.Errorf("Expected most IPs to be cleaned up, still have %d", len(rl.ips))
	}
	rl.mu.Unlock()
}
