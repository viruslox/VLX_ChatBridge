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
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/chatflow/announcer"
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
	config     *config.Config
	controller module.Controller
	mux        *http.ServeMux

	regOnce sync.Once    // routes are registered on the shared mux exactly once
	mu      sync.RWMutex // guards the fields below (rebuilt across Start/Stop)
	started bool
	logger  *zap.Logger
	hub     *websocket.Hub
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

// state returns a consistent snapshot of the runtime fields. Handlers use this
// so a request that arrives while the module is stopped (or mid-restart) sees a
// coherent view and can respond 503 instead of dereferencing a nil/rebuilt ref.
func (m *Module) state() (bool, *websocket.Hub, *zap.Logger, *twitch.Client) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started, m.hub, m.logger, m.twitchClient
}

// Start initializes and starts the ChatFlow components.
func (m *Module) Start() error {
	log.Println("[ChatFlow] Starting module...")

	// Register routes exactly once. On a second Start (runtime re-enable) this
	// block is skipped, so the shared mux is never re-registered (which panics).
	// The handlers gate on state(), so they are safe before the first Start
	// finishes publishing resources and after a Stop tears them down.
	m.regOnce.Do(func() {
		m.mux.HandleFunc("/api/modules/", m.handleModuleToggle)
		m.mux.HandleFunc("/api/alert", m.handleAlert)

		wsPath := m.config.Server.WebsocketPath
		if wsPath == "" {
			wsPath = "/ws"
		}
		if !strings.HasPrefix(wsPath, "/") {
			wsPath = "/" + wsPath
		}
		m.mux.HandleFunc(wsPath, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != wsPath {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
			started, hub, logger, _ := m.state()
			if !started || hub == nil {
				http.Error(w, "ChatFlow not running", http.StatusServiceUnavailable)
				return
			}
			allowedOrigins := m.config.Server.AllowedOrigins
			if len(allowedOrigins) == 0 {
				allowedOrigins = []string{"*"}
			}
			websocket.ServeWs(hub, logger, allowedOrigins, w, r)
		})

		m.mux.HandleFunc("/webhooks/twitch", func(w http.ResponseWriter, r *http.Request) {
			started, _, _, tc := m.state()
			if !started || tc == nil {
				http.Error(w, "ChatFlow not running", http.StatusServiceUnavailable)
				return
			}
			tc.HandleEventSubCallback(w, r)
		})

		m.mux.HandleFunc("/static/alerts_overlay.html", func(w http.ResponseWriter, r *http.Request) {
			m.serveTemplate(w, r, "alerts_overlay.html")
		})
		m.mux.HandleFunc("/static/chat_overlay.html", func(w http.ResponseWriter, r *http.Request) {
			m.serveTemplate(w, r, "chat_overlay.html")
		})
		m.mux.HandleFunc("/static/emotes_overlay.html", func(w http.ResponseWriter, r *http.Request) {
			m.serveTemplate(w, r, "emotes_overlay.html")
		})
		m.mux.HandleFunc("/static/gps_overlay.html", func(w http.ResponseWriter, r *http.Request) {
			m.serveTemplate(w, r, "gps_overlay.html")
		})
		m.mux.HandleFunc("/static/scenes_overlay.html", func(w http.ResponseWriter, r *http.Request) {
			m.serveTemplate(w, r, "scenes_overlay.html")
		})

		baseDir := m.config.ChatBridgeDIR
		if baseDir == "" {
			baseDir = "."
		}
		staticPath := filepath.Join(baseDir, "static")
		m.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))
	})

	// Build resources into locals; publish them (and mark started) atomically at
	// the end so no handler observes a half-initialized module.
	logger, _ := zap.NewProduction()

	dbConn, err := database.NewConnection(m.config.Database, logger)
	if err != nil {
		return fmt.Errorf("[ChatFlow] Database connection error: %w", err)
	}

	hub := websocket.NewHub(logger)
	go hub.Run()

	chatStaticDir := filepath.Join(m.config.ChatBridgeDIR, "static", "chat")
	cmdMap, err := twitch.ScanAudioCommands(chatStaticDir, logger)
	if err != nil {
		logger.Warn("Audio commands scan failed", zap.Error(err))
	}

	announcementsMap, err := twitch.ScanAnnouncements(chatStaticDir, logger)
	if err != nil {
		logger.Warn("Announcements scan failed", zap.Error(err))
	}

	if bool(m.config.Announce.Enable) {
		u := m.config.Announce.WebhookURL
		if u == "" || !strings.HasPrefix(u, "https://discord.com/api/webhooks/") {
			logger.Warn("Announce is enabled but webhook_url is empty or not a Discord webhook URL; announcements will not be sent",
				zap.String("webhook_url", u))
		}
	}

	ann := announcer.New(announcer.Config{
		Enable:          bool(m.config.Announce.Enable),
		WebhookURL:      m.config.Announce.WebhookURL,
		Username:        m.config.Announce.Username,
		AvatarURL:       m.config.Announce.AvatarURL,
		CombineWindow:   time.Duration(m.config.Announce.CombineWindow) * time.Second,
		TwitchEnabled:   bool(m.config.Announce.Twitch.Enable),
		YouTubeEnabled:  bool(m.config.Announce.YouTube.Enable),
		MessageTemplate: m.config.Announce.MessageTemplate,
		EndEnable:       bool(m.config.Announce.EndEnable),
		EndTemplate:     m.config.Announce.EndTemplate,
		EmbedEnable:     bool(m.config.Announce.EmbedEnable),
	}, logger)

	if dbConn != nil {
		ann.SetStore(dbConn)
		if err := dbConn.PruneAnnounceLog(7 * 24 * time.Hour); err != nil {
			logger.Warn("Failed to prune announce_log", zap.Error(err))
		}
	}

	twitchClient, err := twitch.NewClient(m.config, []string{m.config.Twitch.ChannelName}, m.config.Server.BaseURL, hub, dbConn, logger)
	if err != nil {
		logger.Error("Twitch Client init failed", zap.Error(err))
	}
	if twitchClient != nil {
		twitchClient.SetAnnouncer(ann)
		if err := twitchClient.StartMonitoring([]string{m.config.Twitch.ChannelName}); err != nil {
			logger.Error("Twitch monitoring failed", zap.Error(err))
		}
	}

	var chatClient *twitch.ChatClient
	if cmdMap != nil && (m.config.Twitch.Chat.BotUsername != "" || m.config.Twitch.Chat.ChannelToJoin != "" || m.config.Twitch.ChannelName != "") {
		chatClient = twitch.NewChatClient(m.config, hub, dbConn, cmdMap, announcementsMap, logger)
		chatClient.Start()
	}

	youtubeClient, err := youtube.NewClient(m.config, hub, dbConn, cmdMap, logger)
	if err != nil {
		logger.Error("YouTube Client init failed", zap.Error(err))
	}
	if youtubeClient != nil {
		youtubeClient.SetAnnouncer(ann)
		youtubeClient.Start()
	}

	if chatClient != nil && youtubeClient != nil {
		youtubeClient.SetPresence(chatClient.Presence())
	}

	m.mu.Lock()
	m.logger = logger
	m.db = dbConn
	m.hub = hub
	m.twitchClient = twitchClient
	m.chatClient = chatClient
	m.youtubeClient = youtubeClient
	m.started = true
	m.mu.Unlock()

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
		err := audio.DecodeMediaToPCM("test_alert", fullPath, bool(streamingEnabled), bool(discordEnabled), m.config.Overlay.Alerts.Volume)
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

	m.mu.Lock()
	m.started = false
	chatClient := m.chatClient
	youtubeClient := m.youtubeClient
	db := m.db
	hub := m.hub
	twitchClient := m.twitchClient
	m.chatClient = nil
	m.youtubeClient = nil
	m.twitchClient = nil
	m.hub = nil
	m.db = nil
	m.logger = nil
	m.mu.Unlock()

	if chatClient != nil {
		chatClient.Stop()
	}

	if youtubeClient != nil {
		youtubeClient.Stop()
	}

	if twitchClient != nil {
		twitchClient.Stop()
	}

	if hub != nil {
		// Stop the WebSocket hub alongside other teardowns.
		hub.Stop()
	}

	if db != nil {
		db.Close()
		log.Println("[ChatFlow] Database disconnected successfully.")
	}

	log.Println("[ChatFlow] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "ChatFlow"
}

func (m *Module) serveTemplate(w http.ResponseWriter, r *http.Request, filename string) {
	started, _, logger, _ := m.state()
	if !started || logger == nil {
		http.Error(w, "ChatFlow not running", http.StatusServiceUnavailable)
		return
	}

	websocketPath := m.config.Server.WebsocketPath
	pathPrefix := m.config.Server.PathPrefix
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-IP") == "" && r.Header.Get("X-Forwarded-Host") == "" {
		pathPrefix = ""
	}

	// Determine volume based on template filename, default to 100 if not set or invalid
	vol := 100
	switch filename {
	case "alerts_overlay.html":
		vol = m.config.Overlay.Alerts.Volume
	case "chat_overlay.html":
		vol = m.config.Overlay.Chat.Volume
	case "scenes_overlay.html":
		vol = m.config.Overlay.Scenes.Volume
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
		GPSEventType  string
	}{
		WebsocketPath: publicWsPath,
		AssetPrefix:   publicAssetPrefix,
		Volume:        vol,
		GPSEventType:  m.config.Overlay.GPS.EventType,
	}

	baseDir := m.config.ChatBridgeDIR
	if baseDir == "" {
		baseDir = "."
	}
	fp := filepath.Join(baseDir, "static", filename)
	tmpl, err := template.ParseFiles(fp)
	if err != nil {
		logger.Error("Failed to parse template", zap.String("file", filename), zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		logger.Error("Failed to execute template", zap.String("file", filename), zap.Error(err))
	}
}
