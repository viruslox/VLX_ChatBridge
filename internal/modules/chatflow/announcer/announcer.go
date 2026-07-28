// Package announcer sends a "went live" notification to a Discord channel via
// an incoming webhook. It is deliberately platform-agnostic: both the Twitch
// EventSub path and the YouTube polling path call NotifyLive, and the announcer
// itself owns all Discord-specific concerns.
//
// "Combined if both live" is implemented as a short coalescing window. The first
// platform to report live opens a timer (CombineWindow). If the second platform
// reports before the timer fires, both are merged into a single message.
// Otherwise, whatever platforms are live when the timer expires are announced
// together. This behaves correctly for the Twitch-only, YouTube-only, and
// both-live cases without ever blocking on a platform that may never go live.
//
// A per-session fire-once guard prevents the YouTube polling loop (and Twitch
// reconnect storms) from re-announcing. The guard is released via Reset, which
// the callers invoke when a stream genuinely ends (Twitch stream.offline) or
// when the live signal is lost (YouTube live-chat-id lost / poll failure).
package announcer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Platform identifies the source of a go-live event.
type Platform string

const (
	PlatformTwitch  Platform = "Twitch"
	PlatformYouTube Platform = "YouTube"
)

// Config holds the resolved announce settings.
type Config struct {
	Enable          bool
	WebhookURL      string
	Username        string
	AvatarURL       string
	CombineWindow   time.Duration
	TwitchEnabled   bool
	YouTubeEnabled  bool
	MessageTemplate string // go-live message; placeholders {platforms} {title} {url}

	// End-of-stream announce. Independent of the go-live path: fires immediately
	// per-platform (no coalescing). EndTemplate placeholders: {platform} {url}.
	EndEnable   bool
	EndTemplate string
}

// liveInfo captures the per-platform details reported at go-live time.
type liveInfo struct {
	title string
	url   string
	at    time.Time
}

// Announcer coordinates go-live notifications.
type Announcer struct {
	cfg    Config
	logger *zap.Logger
	client *http.Client

	mu      sync.Mutex
	pending map[Platform]liveInfo // platforms currently considered live this session
	timer   *time.Timer           // coalescing timer; non-nil while a window is open
	fired   bool                  // fire-once guard for the current live session
}

// New builds an Announcer. A nil-safe zero value is returned when disabled so
// callers may hold a possibly-nil pointer and always call methods on it.
func New(cfg Config, logger *zap.Logger) *Announcer {
	if cfg.CombineWindow <= 0 {
		cfg.CombineWindow = 45 * time.Second
	}
	if strings.TrimSpace(cfg.MessageTemplate) == "" {
		cfg.MessageTemplate = "\U0001F534 Live now on {platforms}: {title}\n{url}"
	}
	if strings.TrimSpace(cfg.EndTemplate) == "" {
		cfg.EndTemplate = "\u26AB {platform} stream has ended."
	}
	return &Announcer{
		cfg:     cfg,
		logger:  logger,
		client:  &http.Client{Timeout: 10 * time.Second},
		pending: make(map[Platform]liveInfo),
	}
}

// platformEnabled reports whether a given platform is allowed to announce.
func (a *Announcer) platformEnabled(p Platform) bool {
	switch p {
	case PlatformTwitch:
		return a.cfg.TwitchEnabled
	case PlatformYouTube:
		return a.cfg.YouTubeEnabled
	default:
		return false
	}
}

// NotifyLive records that a platform went live and (re)arms the coalescing
// window. Safe to call from multiple goroutines. No-op when the announcer is
// nil, disabled, missing a webhook URL, or the platform is disabled.
func (a *Announcer) NotifyLive(p Platform, title, url string) {
	if a == nil || !a.cfg.Enable || a.cfg.WebhookURL == "" {
		return
	}
	if !a.platformEnabled(p) {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Already announced this session: record the platform (so an offline reset
	// is coherent) but do not re-fire.
	if a.fired {
		a.pending[p] = liveInfo{title: title, url: url, at: time.Now()}
		return
	}

	a.pending[p] = liveInfo{title: title, url: url, at: time.Now()}

	if a.timer == nil {
		a.timer = time.AfterFunc(a.cfg.CombineWindow, a.flush)
		a.logger.Info("Announce window opened",
			zap.String("platform", string(p)),
			zap.Duration("window", a.cfg.CombineWindow))
	}
}

// Reset clears the live state for a platform. When no platforms remain live the
// fire-once guard is released so the next genuine go-live re-announces.
func (a *Announcer) Reset(p Platform) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.pending, p)
	if len(a.pending) == 0 {
		a.fired = false
		if a.timer != nil {
			a.timer.Stop()
			a.timer = nil
		}
		a.logger.Info("Announce session reset", zap.String("platform", string(p)))
	}
}

// NotifyEnd sends an immediate, per-platform end-of-stream announce (no
// coalescing), then clears session state for that platform via Reset so the
// next genuine go-live re-arms. The platform's live URL is recovered from the
// pending record when available. No-op when the announcer is nil, disabled,
// end-announce is disabled, or the platform is disabled.
func (a *Announcer) NotifyEnd(p Platform) {
	if a == nil || !a.cfg.Enable || !a.cfg.EndEnable || a.cfg.WebhookURL == "" {
		return
	}
	if !a.platformEnabled(p) {
		return
	}

	a.mu.Lock()
	// Only announce an end if we actually announced this session going live,
	// and only once per platform-end (presence of a pending record gates it).
	info, live := a.pending[p]
	shouldSend := a.fired && live
	url := ""
	if live {
		url = info.url
	}
	tmpl := a.cfg.EndTemplate
	a.mu.Unlock()

	if shouldSend {
		msg := renderEndTemplate(tmpl, string(p), url)
		a.send(msg)
	}

	// Always release session state for this platform.
	a.Reset(p)
}

// renderEndTemplate performs {platform}/{url} substitution for end announces.
func renderEndTemplate(tmpl, platform, url string) string {
	r := strings.NewReplacer(
		"{platform}", platform,
		"{url}", url,
	)
	return r.Replace(tmpl)
}

// flush is invoked by the coalescing timer. It composes and sends one message
// for all platforms live at expiry, then marks the session as fired.
func (a *Announcer) flush() {
	a.mu.Lock()
	a.timer = nil
	if a.fired || len(a.pending) == 0 {
		a.mu.Unlock()
		return
	}
	a.fired = true

	// Deterministic ordering: earliest go-live first (that platform owns {title}).
	type entry struct {
		p    Platform
		info liveInfo
	}
	entries := make([]entry, 0, len(a.pending))
	for p, info := range a.pending {
		entries = append(entries, entry{p: p, info: info})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].info.at.Before(entries[j].info.at)
	})

	names := make([]string, 0, len(entries))
	urls := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, string(e.p))
		if e.info.url != "" {
			urls = append(urls, e.info.url)
		}
	}
	title := entries[0].info.title
	cfg := a.cfg
	a.mu.Unlock()

	msg := renderTemplate(cfg.MessageTemplate, strings.Join(names, " + "), title, strings.Join(urls, "\n"))
	a.send(msg)
}

// renderTemplate performs simple {placeholder} substitution.
func renderTemplate(tmpl, platforms, title, url string) string {
	r := strings.NewReplacer(
		"{platforms}", platforms,
		"{title}", title,
		"{url}", url,
	)
	return r.Replace(tmpl)
}

// discordWebhookPayload is the subset of the Discord webhook body we set.
type discordWebhookPayload struct {
	Content   string `json:"content"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// send performs the outbound webhook POST. Fire-and-forget: failures are logged,
// not retried.
func (a *Announcer) send(content string) {
	payload := discordWebhookPayload{
		Content:   content,
		Username:  a.cfg.Username,
		AvatarURL: a.cfg.AvatarURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("Announce marshal failed", zap.Error(err))
		return
	}

	req, err := http.NewRequest(http.MethodPost, a.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		a.logger.Error("Announce request build failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		a.logger.Error("Announce POST failed", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		a.logger.Error("Announce POST non-2xx",
			zap.Int("status", resp.StatusCode),
			zap.String("detail", fmt.Sprintf("webhook returned %d", resp.StatusCode)))
		return
	}
	a.logger.Info("Announce sent", zap.String("content", content))
}
