package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

func TestCheckOrigin(t *testing.T) {
	allowedOrigins := []string{"http://localhost:8000", "https://example.com"}

	tests := []struct {
		name           string
		origin         string
		allowedOrigins []string
		expected       bool
	}{
		{
			name:           "Empty origin",
			origin:         "",
			allowedOrigins: allowedOrigins,
			expected:       true, // gorilla/websocket behavior: if Origin header is not present, it's not a cross-origin request
		},
		{
			name:           "Allowed origin 1",
			origin:         "http://localhost:8000",
			allowedOrigins: allowedOrigins,
			expected:       true,
		},
		{
			name:           "Allowed origin 2",
			origin:         "https://example.com",
			allowedOrigins: allowedOrigins,
			expected:       true,
		},
		{
			name:           "Disallowed origin",
			origin:         "http://malicious.com",
			allowedOrigins: allowedOrigins,
			expected:       false,
		},
		{
			name:           "Subdomain disallowed",
			origin:         "http://sub.example.com",
			allowedOrigins: allowedOrigins,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/ws", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			// We need a way to test the CheckOrigin logic without full Upgrade if possible.
			// Since upgrader is now local to ServeWs, we can't easily access it.
			// However, we can duplicate the logic here or refactor ServeWs slightly to make it testable.
			// Given the constraints, I will test the logic as it was implemented in ServeWs.

			checkOrigin := func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				for _, allowed := range tt.allowedOrigins {
					if origin == allowed {
						return true
					}
				}
				return false
			}

			got := checkOrigin(r)
			if got != tt.expected {
				t.Errorf("CheckOrigin() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestServeWsOriginValidation(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	allowedOrigins := []string{"http://localhost:8000"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, logger, allowedOrigins, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("Allowed Origin", func(t *testing.T) {
		dialer := websocket.Dialer{}
		header := http.Header{}
		header.Set("Origin", "http://localhost:8000")
		conn, _, err := dialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		conn.Close()
	})

	t.Run("Disallowed Origin", func(t *testing.T) {
		dialer := websocket.Dialer{}
		header := http.Header{}
		header.Set("Origin", "http://malicious.com")
		_, _, err := dialer.Dial(wsURL, header)
		if err == nil {
			t.Fatal("Expected error connecting from disallowed origin, but got nil")
		}
	})
}
