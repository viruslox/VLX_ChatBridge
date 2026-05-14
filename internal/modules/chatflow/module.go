package chatflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/chatflow/twitch"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"
	"VLX_ChatBridge/internal/modules/chatflow/youtube"
)

// Module represents the ChatFlow component.
type Module struct {
	config     *config.Config
	controller module.Controller
	server     *http.Server
	wsManager  *websocket.WebSocketManager
	twitch     *twitch.TwitchClient
	youtube    *youtube.YouTubeClient
}

// NewModule creates a new instance of the ChatFlow module.
func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
	}
}

// Start initializes and starts the ChatFlow components.
func (m *Module) Start() error {
	log.Println("[ChatFlow] Starting module...")

	mux := http.NewServeMux()

	// API endpoint to toggle modules
	mux.HandleFunc("/api/modules/", m.handleModuleToggle)

	port := m.config.Server.Port
	if port == "" {
		port = "8000"
	}

	m.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("[ChatFlow] HTTP server listening on %s", m.server.Addr)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ChatFlow] HTTP server error: %v", err)
		}
	}()

	// Initialize WebSockets, Twitch, YouTube
	m.wsManager = websocket.NewManager()
	if err := m.wsManager.Start(); err != nil {
		log.Printf("[ChatFlow] WebSocket manager error: %v", err)
	}

	m.twitch = twitch.NewClient()
	if err := m.twitch.Connect(); err != nil {
		log.Printf("[ChatFlow] Twitch client error: %v", err)
	}

	m.youtube = youtube.NewClient()
	if err := m.youtube.Connect(); err != nil {
		log.Printf("[ChatFlow] YouTube client error: %v", err)
	}

	log.Println("[ChatFlow] Started successfully.")
	return nil
}

// handleModuleToggle handles POST requests to start or stop modules dynamically.
// Example: POST /api/modules/AudioBridge/start
func (m *Module) handleModuleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/modules/"), "/")
	if len(parts) != 2 {
		http.Error(w, "Invalid path format. Expected /api/modules/{name}/{action}", http.StatusBadRequest)
		return
	}

	moduleName := parts[0]
	action := parts[1]

	switch action {
	case "start":
		go func() {
			if err := m.controller.StartModule(moduleName); err != nil {
				log.Printf("Failed to start module %s: %v", moduleName, err)
			}
		}()
	case "stop":
		go func() {
			if err := m.controller.StopModule(moduleName); err != nil {
				log.Printf("Failed to stop module %s: %v", moduleName, err)
			}
		}()
	default:
		http.Error(w, "Invalid action. Use 'start' or 'stop'", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": fmt.Sprintf("Initiated %s for module %s", action, moduleName),
	})
}

// Stop cleanly shuts down the ChatFlow components.
func (m *Module) Stop() error {
	log.Println("[ChatFlow] Stopping module...")

	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			log.Printf("[ChatFlow] HTTP server shutdown error: %v", err)
		}
	}

	if m.twitch != nil {
		m.twitch.Disconnect()
	}

	if m.youtube != nil {
		m.youtube.Disconnect()
	}

	if m.wsManager != nil {
		m.wsManager.Stop()
	}

	log.Println("[ChatFlow] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "ChatFlow"
}
