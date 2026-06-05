package chatflow

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/chatflow/audio"
	"VLX_ChatBridge/internal/modules/chatflow/database"
	"VLX_ChatBridge/internal/modules/chatflow/twitch"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"
	"VLX_ChatBridge/internal/modules/chatflow/youtube"

	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

// Module represents the ChatFlow component.
type Module struct {
	config        *config.Config
	controller    module.Controller
	mux           *http.ServeMux
	logger        *zap.Logger
	hub           *websocket.Hub
	twitchClient  *twitch.Client
	chatClient    *twitch.ChatClient
	youtubeClient *youtube.Client
	db            *database.DB
}

// NewModule creates a new instance of the ChatFlow module.
func NewModule(cfg *config.Config, ctrl module.Controller, mux *http.ServeMux) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
		mux:        mux,
	}
}

// Start initializes and starts the ChatFlow components.
func (m *Module) Start() error {
	log.Println("[ChatFlow] Starting module...")

	// API endpoint to toggle modules
	m.mux.HandleFunc("/api/modules/", m.handleModuleToggle)

	// API endpoint to simulate an alert
	m.mux.HandleFunc("/api/alert", m.handleAlert)

	// WebSocket handler
	wsPath := m.config.Server.WebsocketPath
	if wsPath == "" {
		wsPath = "/ws"
	}
	// Make sure the path starts with a slash
	if !strings.HasPrefix(wsPath, "/") {
		wsPath = "/" + wsPath
	}
	m.mux.HandleFunc(wsPath, func(w http.ResponseWriter, r *http.Request) {
		allowedOrigins := m.config.Server.AllowedOrigins
		if len(allowedOrigins) == 0 {
			allowedOrigins = []string{"*"}
		}
		websocket.ServeWs(m.hub, m.logger, allowedOrigins, w, r)
	})

	// Twitch webhook handler
	m.mux.HandleFunc("/webhooks/twitch", func(w http.ResponseWriter, r *http.Request) {
		if m.twitchClient != nil {
			m.twitchClient.HandleEventSubCallback(w, r)
		}
	})

	// Template routes
	m.mux.HandleFunc("/static/alerts_overlay.html", func(w http.ResponseWriter, r *http.Request) {
		m.serveTemplate(w, "alerts_overlay.html")
	})
	m.mux.HandleFunc("/static/chat_overlay.html", func(w http.ResponseWriter, r *http.Request) {
		m.serveTemplate(w, "chat_overlay.html")
	})
	m.mux.HandleFunc("/static/emotes_overlay.html", func(w http.ResponseWriter, r *http.Request) {
		m.serveTemplate(w, "emotes_overlay.html")
	})
	m.mux.HandleFunc("/static/gps_overlay.html", func(w http.ResponseWriter, r *http.Request) {
		m.serveTemplate(w, "gps_overlay.html")
	})

	// Static file server for overlays
	baseDir := m.config.ChatBridgeDIR
	if baseDir == "" {
		baseDir = "."
	}
	staticPath := filepath.Join(baseDir, "static")
	m.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	// Initialize Database connection
	logger, _ := zap.NewProduction()
	m.logger = logger

	dbConn, err := database.NewConnection(m.config.Database, logger)
	if err != nil {
		return fmt.Errorf("[ChatFlow] Database connection error: %w", err)
	}
	m.db = dbConn

	// Initialize WebSockets
	hub := websocket.NewHub(logger)
	go hub.Run()
	m.hub = hub

	chatStaticDir := filepath.Join(m.config.ChatBridgeDIR, "static", "chat")
	cmdMap, err := twitch.ScanAudioCommands(chatStaticDir, logger)
	if err != nil {
		logger.Warn("Audio commands scan failed", zap.Error(err))
	}

	announcementsMap, err := twitch.ScanAnnouncements(chatStaticDir, logger)
	if err != nil {
		logger.Warn("Announcements scan failed", zap.Error(err))
	}

	twitchClient, err := twitch.NewClient(m.config, []string{m.config.Twitch.ChannelName}, m.config.Server.BaseURL, hub, m.db, logger)
	if err != nil {
		logger.Error("Twitch Client init failed", zap.Error(err))
	}
	m.twitchClient = twitchClient

	if m.twitchClient != nil {
		if err := m.twitchClient.StartMonitoring([]string{m.config.Twitch.ChannelName}); err != nil {
			logger.Error("Twitch monitoring failed", zap.Error(err))
		}
	}

	var chatClient *twitch.ChatClient
	if cmdMap != nil && (m.config.Twitch.Chat.BotUsername != "" || m.config.Twitch.Chat.ChannelToJoin != "" || m.config.Twitch.ChannelName != "") {
		chatClient = twitch.NewChatClient(m.config, hub, m.db, cmdMap, announcementsMap, logger)
		chatClient.Start()
	}
	m.chatClient = chatClient

	youtubeClient, err := youtube.NewClient(m.config, hub, m.db, cmdMap, logger)
	if err != nil {
		logger.Error("YouTube Client init failed", zap.Error(err))
	}
	m.youtubeClient = youtubeClient
	if m.youtubeClient != nil {
		m.youtubeClient.Start()
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
		"status":  "success",
		"message": fmt.Sprintf("Initiated %s for module %s", action, moduleName),
	})
}

// handleAlert handles POST requests to trigger an alert.
// Example: POST /api/alert
func (m *Module) handleAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		streamingEnabled := m.config.Overlay.Alerts.Streaming
		discordEnabled := m.config.Overlay.Alerts.Discord
		fullPath := filepath.Join(m.config.ChatBridgeDIR, "static", "alerts", "alert.mp3")
		connectorEnabled := bool(m.config.Modules.ConnectorEnabled) && bool(m.config.Connector.IPCAudioOut)
		err := audio.DecodeMediaToPCM("test_alert", fullPath, bool(streamingEnabled), bool(discordEnabled), connectorEnabled)
		if err != nil {
			log.Printf("[ChatFlow] Error decoding alert: %v", err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Alert triggered",
	})
}

// Stop cleanly shuts down the ChatFlow components.
func (m *Module) Stop() error {
	log.Println("[ChatFlow] Stopping module...")

	if m.chatClient != nil {
		m.chatClient.Stop()
	}

	if m.youtubeClient != nil {
		m.youtubeClient.Stop()
	}

	if m.db != nil {
		m.db.Close()
		log.Println("[ChatFlow] Database disconnected successfully.")
	}

	log.Println("[ChatFlow] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "ChatFlow"
}

func (m *Module) serveTemplate(w http.ResponseWriter, filename string) {
	websocketPath := m.config.Server.WebsocketPath
	pathPrefix := m.config.Server.PathPrefix

	// Determine volume based on template filename, default to 100 if not set or invalid
	vol := 100
	switch filename {
	case "alerts_overlay.html":
		vol = m.config.Overlay.Alerts.Volume
	case "chat_overlay.html":
		vol = m.config.Overlay.Chat.Volume
	}

	publicWsPath := path.Join(pathPrefix, websocketPath)
	publicAssetPrefix := pathPrefix

	if vol < 0 {
		vol = 100
	}

	data := struct {
		WebsocketPath string
		AssetPrefix   string
		Volume        int // Injected volume
	}{
		WebsocketPath: publicWsPath,
		AssetPrefix:   publicAssetPrefix,
		Volume:        vol,
	}

	baseDir := m.config.ChatBridgeDIR
	if baseDir == "" {
		baseDir = "."
	}
	fp := filepath.Join(baseDir, "static", filename)
	tmpl, err := template.ParseFiles(fp)
	if err != nil {
		m.logger.Error("Failed to parse template", zap.String("file", filename), zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		m.logger.Error("Failed to execute template", zap.String("file", filename), zap.Error(err))
	}
}
