package twitch

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/modules/chatflow/database"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

// mockTransport intercepts HTTP requests and returns predefined responses
type mockTransport struct {
	roundTripFunc func(req *http.Request) *http.Response
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req), nil
}

// setupTestDB creates a mock database connection
func setupTestDB(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}

	logger := zap.NewNop()
	return database.ExportedForTesting(db, logger), mock
}

func TestNewClient_Success(t *testing.T) {
	// 1. Setup mock DB
	db, mock := setupTestDB(t)

	// Expectations for user token check
	mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
		WithArgs("mock_user_id").
		WillReturnRows(sqlmock.NewRows([]string{"access_token", "refresh_token", "expires_at"}).
			AddRow("db_token", "db_refresh", time.Now().Add(1*time.Hour)))

	// 2. Setup mock HTTP Client for Helix
	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			urlStr := req.URL.String()

			// Mock App Access Token request
			if strings.Contains(urlStr, "oauth2/token") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token", "expires_in": 3600}`)),
					Header:     make(http.Header),
				}
			}

			// Mock GetUsers request
			if strings.Contains(urlStr, "helix/users") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data": [{"id": "mock_user_id", "login": "test_channel"}]}`)),
					Header:     make(http.Header),
				}
			}

			t.Fatalf("Unexpected request: %s", urlStr)
			return nil
		},
	}
	defer func() { http.DefaultClient.Transport = nil }() // reset

	// 3. Setup dependencies
	cfg := config.TwitchConfig{
		ClientID:     "mock_client_id",
		ClientSecret: "mock_client_secret",
	}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)
	go hub.Run() // Start hub to prevent deadlocks if messages are sent

	// 4. Call NewClient
	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"test_channel"}, "http://localhost", hub, db, logger)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestNewClient_AppTokenFailure(t *testing.T) {
	db, _ := setupTestDB(t)

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			if strings.Contains(req.URL.String(), "oauth2/token") {
				return &http.Response{
					StatusCode: 401,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "unauthorized", "message": "Invalid client secret"}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	cfg := config.TwitchConfig{ClientID: "mock", ClientSecret: "mock"}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"test_channel"}, "http://localhost", hub, db, logger)
	if err == nil {
		t.Fatal("Expected error due to app token failure, got nil")
	}
	if client != nil {
		t.Fatal("Expected nil client, got a client instance")
	}
	if !strings.Contains(err.Error(), "failed to generate app access token") {
		t.Errorf("Expected error about app access token, got: %v", err)
	}
}

func TestNewClient_EmptyChannels(t *testing.T) {
	db, _ := setupTestDB(t)

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			if strings.Contains(req.URL.String(), "oauth2/token") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token"}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	cfg := config.TwitchConfig{ClientID: "mock", ClientSecret: "mock"}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{}, "http://localhost", hub, db, logger)
	if err == nil {
		t.Fatal("Expected error due to empty monitoring channels, got nil")
	}
	if client != nil {
		t.Fatal("Expected nil client")
	}
	if !strings.Contains(err.Error(), "monitoring channels list is empty") {
		t.Errorf("Expected specific error, got: %v", err)
	}
}

func TestNewClient_FallbackToken(t *testing.T) {
	db, mock := setupTestDB(t)

	// User not found in DB
	mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
		WithArgs("mock_user_id").
		WillReturnError(sql.ErrNoRows)

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			urlStr := req.URL.String()
			if strings.Contains(urlStr, "oauth2/token") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token"}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "helix/users") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data": [{"id": "mock_user_id", "login": "test_channel"}]}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "oauth2/validate") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"client_id": "mock_client", "login": "test_channel", "user_id": "mock_user_id"}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	cfg := config.TwitchConfig{
		ClientID:        "mock",
		ClientSecret:    "mock",
	}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)
	go hub.Run()

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"test_channel"}, "http://localhost", hub, db, logger)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestNewClient_InvalidToken(t *testing.T) {
	db, mock := setupTestDB(t)

	// User not found in DB
	mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
		WithArgs("mock_user_id").
		WillReturnError(sql.ErrNoRows)

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			urlStr := req.URL.String()
			if strings.Contains(urlStr, "oauth2/token") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token"}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "helix/users") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data": [{"id": "mock_user_id", "login": "test_channel"}]}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	// Empty UserAccessToken -> falls back but fails as no config token exists
	cfg := config.TwitchConfig{ClientID: "mock", ClientSecret: "mock"}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)
	go hub.Run()

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"test_channel"}, "http://localhost", hub, db, logger)
	if err != nil {
		// NewClient does NOT return an error here, it just logs a warning
		t.Fatalf("Expected no error (it logs a warning), got %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestNewClient_UserResolveFailure(t *testing.T) {
	db, _ := setupTestDB(t)

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			urlStr := req.URL.String()
			if strings.Contains(urlStr, "oauth2/token") {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token"}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "helix/users") {
				// Return an empty data array to simulate missing user
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data": []}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	cfg := config.TwitchConfig{ClientID: "mock", ClientSecret: "mock"}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)
	go hub.Run()

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"invalid_channel"}, "http://localhost", hub, db, logger)
	if err != nil {
		t.Fatalf("Expected no error (fails gracefully and logs), got %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}
}

func TestNewClient_TokenRefresh(t *testing.T) {
	db, mock := setupTestDB(t)

	// User token is found in DB, but it's expired
	mock.ExpectQuery("^SELECT access_token, refresh_token, expires_at FROM twitch_credentials WHERE user_id = \\$1$").
		WithArgs("mock_user_id").
		WillReturnRows(sqlmock.NewRows([]string{"access_token", "refresh_token", "expires_at"}).
			AddRow("expired_token", "refresh_token_123", time.Now().Add(-1*time.Hour)))

	// It should then update the DB with the refreshed token
	mock.ExpectExec("^INSERT INTO twitch_credentials").
		WithArgs("mock_user_id", "new_access_token", "new_refresh_token", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	http.DefaultClient.Transport = &mockTransport{
		roundTripFunc: func(req *http.Request) *http.Response {
			urlStr := req.URL.String()
			if strings.Contains(urlStr, "oauth2/token") && strings.Contains(urlStr, "grant_type=client_credentials") {
				// App access token
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "app_token"}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "helix/users") {
				// Resolve User ID
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data": [{"id": "mock_user_id", "login": "test_channel"}]}`)),
					Header:     make(http.Header),
				}
			}
			if strings.Contains(urlStr, "oauth2/token") && strings.Contains(urlStr, "grant_type=refresh_token") {
				// Refresh token response
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token": "new_access_token", "refresh_token": "new_refresh_token", "expires_in": 3600}`)),
					Header:     make(http.Header),
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}
		},
	}
	defer func() { http.DefaultClient.Transport = nil }()

	cfg := config.TwitchConfig{ClientID: "mock", ClientSecret: "mock"}
	logger := zap.NewNop()
	hub := websocket.NewHub(logger)
	go hub.Run()

	client, err := NewClient(&config.Config{Twitch: cfg}, []string{"test_channel"}, "http://localhost", hub, db, logger)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("Expected client, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
