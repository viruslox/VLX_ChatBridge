// Package controlapi exposes the always-on control and status HTTP API used by
// the VLX_ChatBridge web GUI. It runs independently of the Server module so the
// GUI can reach the backend even when that module is disabled.
//
// Toggles are persisted to the settings file (the master); they take effect on
// the next process (re)start, which the GUI can trigger via /api/shutdown under
// a systemd unit configured to relaunch the service. The status view reports the
// live running state from the module manager alongside the persisted settings,
// so any pending (not-yet-applied) change is visible.
package controlapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

// toggle describes one persistable boolean flag exposed to the GUI.
type toggle struct {
	Key   string
	Label string
	Path  []string
}

// moduleToggles are the top-level modules (matching Module.Name()).
var moduleToggles = []toggle{
	{"Server", "Server", []string{"modules", "server_enabled"}},
	{"ChatFlow", "ChatFlow", []string{"modules", "chatflow_enabled"}},
	{"AudioBridge", "AudioBridge", []string{"modules", "audiobridge_enabled"}},
	{"Streaming", "Streaming", []string{"modules", "streaming_enabled"}},
	{"AudioSource", "AudioSource", []string{"modules", "audio_source_enabled"}},
	{"Connector", "Connector", []string{"modules", "connector_enabled"}},
}

// featureToggles are the submodule-level boolean flags.
var featureToggles = []toggle{
	{"overlay.enable", "Overlay (master)", []string{"overlay", "enable"}},
	{"overlay.emotes.html", "Emotes \u2192 HTML", []string{"overlay", "emotes", "html"}},
	{"overlay.alerts.html", "Alerts \u2192 HTML", []string{"overlay", "alerts", "html"}},
	{"overlay.alerts.discord", "Alerts \u2192 Discord", []string{"overlay", "alerts", "discord"}},
	{"overlay.alerts.streaming", "Alerts \u2192 Streaming", []string{"overlay", "alerts", "streaming"}},
	{"overlay.chat.html", "Chat \u2192 HTML", []string{"overlay", "chat", "html"}},
	{"overlay.chat.discord", "Chat \u2192 Discord", []string{"overlay", "chat", "discord"}},
	{"overlay.chat.streaming", "Chat \u2192 Streaming", []string{"overlay", "chat", "streaming"}},
	{"overlay.scenes.enable", "Scenes (enable)", []string{"overlay", "scenes", "enable"}},
	{"overlay.scenes.html", "Scenes \u2192 HTML", []string{"overlay", "scenes", "html"}},
	{"overlay.scenes.discord", "Scenes \u2192 Discord", []string{"overlay", "scenes", "discord"}},
	{"overlay.scenes.streaming", "Scenes \u2192 Streaming", []string{"overlay", "scenes", "streaming"}},
	{"overlay.gps.html", "GPS \u2192 HTML", []string{"overlay", "gps", "html"}},
	{"audio_source.enable", "Audio Source (enable)", []string{"audio_source", "enable"}},
	{"audio_source.discord", "Audio Source \u2192 Discord", []string{"audio_source", "discord"}},
	{"audio_source.streaming", "Audio Source \u2192 Streaming", []string{"audio_source", "streaming"}},
	{"discord.streaming", "Discord capture \u2192 Streaming", []string{"discord", "streaming"}},
	{"streaming.enable", "Streaming (enable)", []string{"streaming", "enable"}},
	{"connector.ipc_control_out", "Connector IPC Out", []string{"connector", "ipc_control_out"}},
	{"announce.enable", "Announce (enable)", []string{"announce", "enable"}},
	{"announce.end_enable", "Announce end message", []string{"announce", "end_enable"}},
	{"announce.embed_enable", "Announce embed", []string{"announce", "embed_enable"}},
	{"announce.twitch.enable", "Announce \u2192 Twitch", []string{"announce", "twitch", "enable"}},
	{"announce.youtube.enable", "Announce \u2192 YouTube", []string{"announce", "youtube", "enable"}},
}

// liveToggleModules are modules whose Start/Stop are safe to cycle in-process,
// so the control API applies their toggles live (no restart needed). The others
// (Server, ChatFlow) share an HTTP mux that is not yet re-entrant, so they are
// persist-and-restart.
var liveToggleModules = map[string]bool{
	"AudioBridge": true,
	"Streaming":   true,
	"AudioSource": true,
	"Connector":   true,
}

func lookup(list []toggle, key string) (toggle, bool) {
	for _, t := range list {
		if t.Key == key {
			return t, true
		}
	}
	return toggle{}, false
}

// Server is the control/status HTTP API.
type Server struct {
	cfgPath  string
	user     string
	pass     string
	manager  *module.Manager
	shutdown func()
	httpSrv  *http.Server

	logUnit string          // systemd unit tailed by the on-demand console
	tickets *ticketManager  // short-lived tickets authorizing the console WS

	mu    sync.Mutex
	dirty bool // a persisted change is awaiting a restart
}

// New builds the control API server. If user is empty, requests are not
// authenticated (the 127.0.0.1 bind is then the only trust boundary).
func New(cfgPath, bindAddr, port, user, pass, logUnit string, mgr *module.Manager, shutdown func()) *Server {
	s := &Server{
		cfgPath:  cfgPath,
		user:     user,
		pass:     pass,
		manager:  mgr,
		shutdown: shutdown,
		logUnit:  logUnit,
		tickets:  newTicketManager(),
	}

	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	if port == "" {
		port = "8760"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/module", s.auth(s.handleModuleToggle))
	mux.HandleFunc("/api/feature", s.auth(s.handleFeatureToggle))
	mux.HandleFunc("/api/shutdown", s.auth(s.handleShutdown))
	// Console: ticket is issued over the authenticated API; the WS route itself
	// is validated by that single-use ticket (a browser cannot set auth headers
	// on a WebSocket handshake).
	mux.HandleFunc("/api/console/ticket", s.auth(s.handleConsoleTicket))
	mux.HandleFunc("/api/console/ws", s.handleConsoleWS)

	s.httpSrv = &http.Server{
		Addr:    bindAddr + ":" + port,
		Handler: mux,
	}
	return s
}

// Start runs the HTTP server in the background.
func (s *Server) Start() {
	go func() {
		log.Printf("[ControlAPI] listening on %s", s.httpSrv.Addr)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ControlAPI] server error: %v", err)
		}
	}()
}

// Stop gracefully shuts the HTTP server down.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.user != "" {
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(s.user)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(s.pass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="VLX_ChatBridge"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type moduleStatus struct {
	Name              string `json:"name"`
	Path              string `json:"path"`
	EnabledInSettings bool   `json:"enabled_in_settings"`
	Running           bool   `json:"running"`
	Toggleable        bool   `json:"toggleable"`
}

type featureStatus struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	State      bool   `json:"state"`
	Toggleable bool   `json:"toggleable"`
}

type statusResponse struct {
	Modules        []moduleStatus  `json:"modules"`
	Features       []featureStatus `json:"features"`
	RestartPending bool            `json:"restart_pending"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Build the set of paths to read in a single file pass.
	paths := make(map[string][]string, len(moduleToggles)+len(featureToggles))
	for _, t := range moduleToggles {
		paths[t.Key] = t.Path
	}
	for _, t := range featureToggles {
		paths[t.Key] = t.Path
	}

	flags, err := config.LoadFlags(s.cfgPath, paths)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	running := make(map[string]bool)
	for _, st := range s.manager.Statuses() {
		running[st.Name] = st.Running
	}

	resp := statusResponse{}
	divergent := false
	for _, t := range moduleToggles {
		en := flags[t.Key]
		run := running[t.Key]
		if en != run {
			divergent = true
		}
		resp.Modules = append(resp.Modules, moduleStatus{
			Name:              t.Key,
			Path:              strings.Join(t.Path, "."),
			EnabledInSettings: en,
			Running:           run,
			Toggleable:        true,
		})
	}
	for _, t := range featureToggles {
		resp.Features = append(resp.Features, featureStatus{
			Key:        t.Key,
			Label:      t.Label,
			State:      flags[t.Key],
			Toggleable: true,
		})
	}

	s.mu.Lock()
	resp.RestartPending = s.dirty || divergent
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

type moduleToggleReq struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleModuleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	var req moduleToggleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if _, ok := lookup(moduleToggles, req.Name); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown module"})
		return
	}
	if err := config.SetModuleEnabled(s.cfgPath, req.Name, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Live-apply for modules whose lifecycle is cycle-safe; the settings file was
	// already updated above, so the running state and the master file stay in
	// sync without a restart. Start/Stop are guarded by IsRunning so a repeated
	// toggle is idempotent.
	if liveToggleModules[req.Name] {
		var applyErr error
		if req.Enabled {
			if !s.manager.IsRunning(req.Name) {
				applyErr = s.manager.StartModule(req.Name)
			}
		} else {
			if s.manager.IsRunning(req.Name) {
				applyErr = s.manager.StopModule(req.Name)
			}
		}
		if applyErr != nil {
			// The change is persisted; only the live application failed.
			writeJSON(w, http.StatusInternalServerError,
				map[string]interface{}{"error": applyErr.Error(), "persisted": true})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "restart_required": false})
		return
	}

	// Server / ChatFlow: persisted now, applied on the next restart.
	s.markDirty()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "restart_required": true})
}

type featureToggleReq struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleFeatureToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	var req featureToggleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	t, ok := lookup(featureToggles, req.Key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown feature"})
		return
	}
	if err := config.SetBoolByPath(s.cfgPath, t.Path, req.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.markDirty()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "restart_required": true})
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	// Trigger a graceful shutdown after the response has been flushed. Under a
	// systemd unit with Restart=always this relaunches the service, applying any
	// persisted toggles.
	go func() {
		time.Sleep(150 * time.Millisecond)
		if s.shutdown != nil {
			s.shutdown()
		}
	}()
}

func (s *Server) markDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
