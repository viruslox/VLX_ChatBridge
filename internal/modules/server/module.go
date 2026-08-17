package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/events"
	"VLX_ChatBridge/internal/core/module"
)

// Module represents the Server component.
type Module struct {
	config     *config.Config
	controller module.Controller
	mux        *http.ServeMux
	server     *http.Server
}

// NewModule creates a new instance of the Server module.
func NewModule(cfg *config.Config, ctrl module.Controller, mux *http.ServeMux) *Module {
	m := &Module{
		config:     cfg,
		controller: ctrl,
		mux:        mux,
	}
	// Register routes once, at construction, so repeated Start/Stop cycles
	// (runtime enable/disable) never re-register on the shared mux -- which would
	// panic. The handler is only reachable while the HTTP listener is up (i.e.
	// while this module is started), so no extra gating is needed.
	mux.HandleFunc("/api/gps", m.handleGPS)
	return m
}

// Start initializes and starts the Server component.
func (m *Module) Start() error {
	log.Println("[Server] Starting module...")

	port := m.config.Server.Port
	if port == "" {
		port = "8000"
	}

	// A fresh http.Server per start (over the already-registered mux) makes the
	// listener safe to bring up and down repeatedly.
	m.server = &http.Server{
		Addr:    ":" + port,
		Handler: m.mux,
	}

	go func() {
		log.Printf("[Server] HTTP server listening on %s", m.server.Addr)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Server] HTTP server error: %v", err)
		}
	}()

	log.Println("[Server] Started successfully.")
	return nil
}

// handleGPS handles the unauthenticated POST /api/gps endpoint for telemetry.
func (m *Module) handleGPS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	eventType := m.config.Overlay.GPS.EventType
	if eventType == "" {
		eventType = "gps"
	}

	wsMessage := map[string]interface{}{
		"type": eventType,
		"data": payload,
	}

	dataBytes, err := json.Marshal(wsMessage)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	select {
	case events.WebSocketBroadcastChan <- dataBytes:
	default:
		log.Println("[Server] WebSocketBroadcastChan is full, dropping GPS update")
	}

	w.WriteHeader(http.StatusOK)
}

// Stop cleanly shuts down the Server component.
func (m *Module) Stop() error {
	log.Println("[Server] Stopping module...")

	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			log.Printf("[Server] HTTP server shutdown error: %v", err)
		}
	}

	log.Println("[Server] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "Server"
}
