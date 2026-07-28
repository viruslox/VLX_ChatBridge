// This file implements the optional rich-embed rendering for announcements.
// It is a self-contained feature: when EmbedEnable is false the announcer uses
// the plain-content path in announcer.go unchanged. When true, go-live and
// end messages are rendered as Discord embeds (coloured side-bar, title,
// fields, footer, timestamp) instead of plain content.
//
// Design: embeds are built from the same data the plain path already has
// (platform names, title, per-platform URLs). To keep the change isolated, the
// embed send goes through sendEmbeds, which mirrors send()'s retry/404 logic by
// delegating to the shared sendBody helper. No coalescing or guard logic lives
// here.
package announcer

import (
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Discord brand-ish colours (decimal RGB) used for the embed side-bar.
const (
	colorTwitch  = 0x9146FF // Twitch purple
	colorYouTube = 0xFF0000 // YouTube red
	colorBoth    = 0x5865F2 // Discord blurple (multi-platform)
	colorEnded   = 0x2B2D31 // dark grey (stream ended)
)

// embed is the subset of a Discord embed object we populate.
type embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []embedField `json:"fields,omitempty"`
	Footer      *embedFooter `json:"footer,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"` // ISO 8601
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type embedFooter struct {
	Text string `json:"text"`
}

// discordEmbedPayload is the webhook body when embeds are used. Content is left
// empty; Discord shows the embed(s).
type discordEmbedPayload struct {
	Username  string  `json:"username,omitempty"`
	AvatarURL string  `json:"avatar_url,omitempty"`
	Embeds    []embed `json:"embeds"`
}

// liveColor picks a side-bar colour based on which platforms are live.
func liveColor(names []string) int {
	if len(names) > 1 {
		return colorBoth
	}
	if len(names) == 1 {
		switch Platform(names[0]) {
		case PlatformTwitch:
			return colorTwitch
		case PlatformYouTube:
			return colorYouTube
		}
	}
	return colorBoth
}

// buildLiveEmbed composes the go-live embed from platform names, the owning
// title, and per-platform URLs (already newline-joinable). primaryURL is the
// first platform's URL (used as the clickable embed title link).
func buildLiveEmbed(names []string, title, joinedURLs, primaryURL string) embed {
	e := embed{
		Title:     "\U0001F534 Live now on " + strings.Join(names, " + "),
		Color:     liveColor(names),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer:    &embedFooter{Text: "VLX ChatBridge"},
	}
	if title != "" {
		e.Description = title
	}
	if primaryURL != "" {
		e.URL = primaryURL
	}
	if joinedURLs != "" {
		e.Fields = append(e.Fields, embedField{
			Name:  "Watch",
			Value: joinedURLs,
		})
	}
	return e
}

// buildEndEmbed composes the end-of-stream embed for a single platform.
func buildEndEmbed(platform, url string) embed {
	e := embed{
		Title:     "\u26AB " + platform + " stream has ended",
		Color:     colorEnded,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer:    &embedFooter{Text: "VLX ChatBridge"},
	}
	if url != "" {
		e.URL = url
	}
	return e
}

// sendLiveEmbed marshals and sends a go-live embed via the shared HTTP core.
func (a *Announcer) sendLiveEmbed(names []string, title, joinedURLs, primaryURL string) {
	e := buildLiveEmbed(names, title, joinedURLs, primaryURL)
	payload := discordEmbedPayload{
		Username:  a.cfg.Username,
		AvatarURL: a.cfg.AvatarURL,
		Embeds:    []embed{e},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("Announce embed marshal failed", zap.Error(err))
		return
	}
	a.sendBody(body, e.Title)
}

// sendEndEmbed marshals and sends an end-of-stream embed for one platform.
func (a *Announcer) sendEndEmbed(platform, url string) {
	e := buildEndEmbed(platform, url)
	payload := discordEmbedPayload{
		Username:  a.cfg.Username,
		AvatarURL: a.cfg.AvatarURL,
		Embeds:    []embed{e},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		a.logger.Error("Announce embed marshal failed", zap.Error(err))
		return
	}
	a.sendBody(body, e.Title)
}
