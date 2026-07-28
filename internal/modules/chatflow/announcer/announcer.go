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
// reconnect storms) from re-announcing within a single process lifetime. The
// guard is released via Reset, which the callers invoke when a stream genuinely
// ends (Twitch stream.offline) or when the live signal is lost (YouTube
// live-chat-id lost / poll failure).
//
// Cross-restart de-duplication: the in-memory guard is lost if ChatBridge
// restarts mid-stream, which would re-announce the same live. To prevent this,
// each go-live carries a stable per-stream identifier (Twitch started_at,
// YouTube videoID). Before announcing, the announcer records (platform, streamID)
// in a persistent Store; if that pair was already recorded, the announce is
// suppressed. See the Store interface.
package announcer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
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

// maxSendAttempts bounds retry attempts on rate-limited/transient webhook POSTs.
const maxSendAttempts = 2

// Store persists which streams have already been announced, enabling
// de-duplication across process restarts. *database.DB satisfies this.
// All methods must be safe to call with a nil receiver-free implementation;
// the announcer treats a nil Store as "no persistence" and falls back to the
// in-memory guard only.
type Store interface {
	// AlreadyAnnounced reports whether (platform, streamID) was previously marked.
	AlreadyAnnounced(platform, streamID string) (bool, error)
	// MarkAnnounced records (platform, streamID) as announced at time.Now().
	MarkAnnounced(platform, streamID string) error
}

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

	// EmbedEnable switches go-live/end messages from plain content to rich
	// Discord embeds (see announcer_embed.go). When false, the plain templates
	// above are used unchanged.
	EmbedEnable bool
}

// liveInfo captures the per-platform details reported at go-live time.
type liveInfo struct {
	title    string
	url      string
	streamID string
	at       time.Time
}

// Announcer coordinates go-live notifications.
type Announcer struct {
	cfg    Config
	logger *zap.Logger
	client *http.Client
	store  Store // optional; nil = in-memory guard only

	mu       sync.Mutex
	pending  map[Platform]liveInfo // platforms currently considered live this session
	timer    *time.Timer           // coalescing timer; non-nil while a window is open
	fired    bool                  // fire-once guard for the current live session
	disabled bool                  // set true after a 404 (dead webhook) to stop retrying
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

// SetStore attaches a persistence layer for cross-restart de-duplication.
// Optional; if never called, the announcer relies on the in-memory guard only.
func (a *Announcer) SetStore(s Store) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.store = s
	a.mu.Unlock()
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
// window. streamID is a stable per-stream identifier (Twitch started_at,
// YouTube videoID) used for cross-restart de-duplication; it may be empty, in
// which case only the in-memory guard applies. Safe to call from multiple
// goroutines. No-op when the announcer is nil, disabled, missing a webhook URL,
// or the platform is disabled.
func (a *Announcer) NotifyLive(p Platform, title, url, streamID string) {
	if a == nil || !a.cfg.Enable || a.cfg.WebhookURL == "" {
		return
	}
	if !a.platformEnabled(p) {
		return
	}

	// Cross-restart de-dup: if this exact stream was already announced in a
	// previous process lifetime, suppress. Done outside the lock (DB call).
	if streamID != "" {
		a.mu.Lock()
		store := a.store
		a.mu.Unlock()
		if store != nil {
			if done, err := store.AlreadyAnnounced(string(p), streamID); err != nil {
				a.logger.Warn("Announce dedup check failed; proceeding",
					zap.String("platform", string(p)), zap.Error(err))
			} else if done {
				a.logger.Info("Go-live already announced in a prior session; suppressing",
					zap.String("platform", string(p)), zap.String("stream_id", streamID))
				// Still record it as live in-session so an end-reset is coherent.
				a.mu.Lock()
				a.fired = true
				a.pending[p] = liveInfo{title: title, url: url, streamID: streamID, at: time.Now()}
				a.mu.Unlock()
				return
			}
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Already announced this session: record the platform (so an offline reset
	// is coherent) but do not re-fire.
	if a.fired {
		a.pending[p] = liveInfo{title: title, url: url, streamID: streamID, at: time.Now()}
		return
	}

	a.pending[p] = liveInfo{title: title, url: url, streamID: streamID, at: time.Now()}

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
		if a.cfg.EmbedEnable {
			a.sendEndEmbed(string(p), url)
		} else {
			msg := renderEndTemplate(tmpl, string(p), url)
			a.send(msg)
		}
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
	store := a.store
	a.mu.Unlock()

	// Persist announced streams for cross-restart de-dup (best-effort).
	if store != nil {
		for _, e := range entries {
			if e.info.streamID == "" {
				continue
			}
			if err := store.MarkAnnounced(string(e.p), e.info.streamID); err != nil {
				a.logger.Warn("Failed to persist announce record",
					zap.String("platform", string(e.p)), zap.Error(err))
			}
		}
	}

	if cfg.EmbedEnable {
		primaryURL := ""
		if len(urls) > 0 {
			primaryURL = urls[0]
		}
		a.sendLiveEmbed(names, title, strings.Join(urls, "\n"), primaryURL)
		return
	}

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

// send performs the outbound webhook POST with bounded retry on rate-limit
// (429) and transient 5xx responses. Fire-and-forget beyond that: it does not
// block the caller's logic and does not retry indefinitely. A 404 permanently
// disables further sends this session (a dead webhook must not be hammered).
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
	a.sendBody(body, content)
}

// sendBody performs the outbound webhook POST for a pre-marshalled JSON body,
// with bounded retry on 429/5xx and permanent disable on 404. desc is used only
// for logging. Shared by the plain-content and embed paths.
func (a *Announcer) sendBody(body []byte, desc string) {
	a.mu.Lock()
	if a.disabled {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	for attempt := 1; attempt <= maxSendAttempts; attempt++ {
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

		status := resp.StatusCode

		// Success.
		if status < 300 {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.logger.Info("Announce sent", zap.String("content", desc))
			return
		}

		// Dead webhook: never retry, disable for this session.
		if status == http.StatusNotFound {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.mu.Lock()
			a.disabled = true
			a.mu.Unlock()
			a.logger.Error("Announce webhook returned 404 (dead URL); disabling announcer for this session. Check announce.webhook_url")
			return
		}

		// Rate-limited: honor Retry-After, then retry (bounded).
		if status == http.StatusTooManyRequests && attempt < maxSendAttempts {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"))
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.logger.Warn("Announce rate-limited (429); retrying after wait",
				zap.Duration("wait", wait), zap.Int("attempt", attempt))
			time.Sleep(wait)
			continue
		}

		// Transient server error: retry (bounded) with a small backoff.
		if status >= 500 && attempt < maxSendAttempts {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			a.logger.Warn("Announce server error; retrying",
				zap.Int("status", status), zap.Int("attempt", attempt))
			time.Sleep(2 * time.Second)
			continue
		}

		// Non-retryable (4xx other than 429) or retries exhausted.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		a.logger.Error("Announce POST non-2xx (giving up)",
			zap.Int("status", status),
			zap.String("detail", fmt.Sprintf("webhook returned %d", status)))
		return
	}
}

// parseRetryAfter interprets a Retry-After header (seconds form) and returns a
// sane, bounded wait. Falls back to 1s when absent or unparseable, and caps at
// 10s so a fire-and-forget announce never blocks a goroutine for long.
func parseRetryAfter(h string) time.Duration {
	const fallback = 1 * time.Second
	const maxWait = 10 * time.Second
	h = strings.TrimSpace(h)
	if h == "" {
		return fallback
	}
	// Discord sends a decimal seconds value (e.g. "1.5").
	if secs, err := strconv.ParseFloat(h, 64); err == nil {
		d := time.Duration(secs * float64(time.Second))
		if d <= 0 {
			return fallback
		}
		if d > maxWait {
			return maxWait
		}
		return d
	}
	return fallback
}
