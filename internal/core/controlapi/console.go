package controlapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ticketManager issues short-lived, single-use tickets. A browser cannot set an
// Authorization header on a WebSocket handshake, so the SPA fetches a ticket
// over the authenticated API and then opens the console WS with ?ticket=.
type ticketManager struct {
	mu      sync.Mutex
	tickets map[string]time.Time
}

func newTicketManager() *ticketManager {
	return &ticketManager{tickets: make(map[string]time.Time)}
}

func (tm *ticketManager) generate() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)

	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cleanup()
	tm.tickets[t] = time.Now().Add(10 * time.Second)
	return t, nil
}

func (tm *ticketManager) validate(t string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cleanup()

	exp, ok := tm.tickets[t]
	if !ok {
		return false
	}
	delete(tm.tickets, t) // single-use
	return time.Now().Before(exp)
}

// cleanup removes expired tickets. Callers must hold tm.mu.
func (tm *ticketManager) cleanup() {
	now := time.Now()
	for t, exp := range tm.tickets {
		if now.After(exp) {
			delete(tm.tickets, t)
		}
	}
}

var consoleUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	},
}

func (s *Server) handleConsoleTicket(w http.ResponseWriter, r *http.Request) {
	if s.logUnit == "" {
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "console not configured (control_api.log_unit is empty)"})
		return
	}
	t, err := s.tickets.generate()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue ticket"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ticket": t})
}

// handleConsoleWS streams the service journal to the client for as long as the
// WebSocket stays open. journalctl -f is spawned per connection and killed the
// moment the client disconnects (via context cancellation), so nothing tails
// the journal while the console box is closed -- keeping idle CPU/RAM at zero.
func (s *Server) handleConsoleWS(w http.ResponseWriter, r *http.Request) {
	if s.logUnit == "" {
		http.Error(w, "console not configured", http.StatusServiceUnavailable)
		return
	}

	ticket := r.URL.Query().Get("ticket")
	if ticket == "" || !s.tickets.validate(ticket) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := consoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ControlAPI] console upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "journalctl",
		"-u", s.logUnit,
		"-n", "200",
		"-f",
		"--no-pager",
		"-o", "cat",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to attach to journal: "+err.Error()))
		return
	}
	// Fold stderr into the same stream so journalctl's own errors (unit not
	// found, permission denied) are visible in the console box.
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start journalctl: "+err.Error()))
		return
	}

	// Detect client disconnect: any read error cancels the context, which kills
	// the journalctl process spawned above.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
			break
		}
	}

	cancel()
	_ = cmd.Wait()
}
