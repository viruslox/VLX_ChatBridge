package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	gotwitch "github.com/gempir/go-twitch-irc/v4"
	"go.uber.org/zap"
)

// Built-in command names. These are handled in code (not file-scanned) but are
// injected into the !commands listing so users can discover them.
const (
	BuiltinFollowage = "followage"
)

// helixFollowResponse maps the Get Channel Followers endpoint payload.
// GET https://api.twitch.tv/helix/channels/followers
type helixFollowResponse struct {
	Total int `json:"total"`
	Data  []struct {
		UserID     string `json:"user_id"`
		UserLogin  string `json:"user_login"`
		UserName   string `json:"user_name"`
		FollowedAt string `json:"followed_at"` // RFC3339
	} `json:"data"`
}

// helixUsersResponse maps the Get Users endpoint payload (login -> id resolution).
type helixUsersResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Login string `json:"login"`
	} `json:"data"`
}

// resolveBroadcasterID returns the numeric ID of the configured broadcaster,
// caching it after the first successful lookup. It uses the broadcaster's own
// valid token (from the DB, keyed by BotID is NOT correct here — we need the
// broadcaster). We resolve by login via Get Users, which any app/user token can call.
func (c *ChatClient) resolveBroadcasterID() (string, error) {
	c.mu.RLock()
	cached := c.broadcasterID
	c.mu.RUnlock()
	if cached != "" {
		return cached, nil
	}

	login := c.config.Twitch.ChannelName
	if login == "" {
		login = c.config.Twitch.Chat.ChannelToJoin
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return "", fmt.Errorf("no broadcaster login configured")
	}

	// Use the bot token (or any valid token) as bearer for the Get Users call.
	token, err := c.helixBearerToken()
	if err != nil {
		return "", err
	}

	reqURL := "https://api.twitch.tv/helix/users?login=" + url.QueryEscape(login)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Client-Id", c.config.Twitch.ClientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get users returned status %d", resp.StatusCode)
	}

	var out helixUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("broadcaster login %q not found", login)
	}

	id := out.Data[0].ID
	c.mu.Lock()
	c.broadcasterID = id
	c.mu.Unlock()
	return id, nil
}

// resolveUserIDByLogin resolves an arbitrary chatter's login to their numeric ID.
func (c *ChatClient) resolveUserIDByLogin(login string) (string, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return "", fmt.Errorf("empty login")
	}
	token, err := c.helixBearerToken()
	if err != nil {
		return "", err
	}
	reqURL := "https://api.twitch.tv/helix/users?login=" + url.QueryEscape(login)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Client-Id", c.config.Twitch.ClientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get users returned status %d", resp.StatusCode)
	}
	var out helixUsersResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("user %q not found", login)
	}
	return out.Data[0].ID, nil
}

// helixBearerToken returns a valid bearer token for Helix calls. It prefers the
// broadcaster token (needed for channels/followers, which requires
// moderator:read:followers on the broadcaster), falling back to the bot token.
func (c *ChatClient) helixBearerToken() (string, error) {
	// Prefer broadcaster: channels/followers with a specific user_id works with
	// the broadcaster's token or a moderator token.
	if bid := c.cachedBroadcasterID(); bid != "" {
		if tok, err := c.getValidToken(bid); err == nil && tok != "" {
			return tok, nil
		}
	}
	if c.config.Twitch.Chat.BotID != "" {
		if tok, err := c.getValidToken(c.config.Twitch.Chat.BotID); err == nil && tok != "" {
			return tok, nil
		}
	}
	return "", fmt.Errorf("no valid Helix token available")
}

func (c *ChatClient) cachedBroadcasterID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.broadcasterID
}

// getFollowedAt returns the followed_at time for targetUserID on the broadcaster
// channel, plus whether they follow at all.
func (c *ChatClient) getFollowedAt(targetUserID string) (time.Time, bool, error) {
	broadcasterID, err := c.resolveBroadcasterID()
	if err != nil {
		return time.Time{}, false, err
	}

	// Broadcaster token is required for channels/followers.
	token, err := c.getValidToken(broadcasterID)
	if err != nil {
		// fall back to whatever bearer we can get
		token, err = c.helixBearerToken()
		if err != nil {
			return time.Time{}, false, err
		}
	}

	reqURL := fmt.Sprintf(
		"https://api.twitch.tv/helix/channels/followers?broadcaster_id=%s&user_id=%s",
		url.QueryEscape(broadcasterID), url.QueryEscape(targetUserID),
	)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return time.Time{}, false, err
	}
	req.Header.Set("Client-Id", c.config.Twitch.ClientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return time.Time{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, false, fmt.Errorf("channels/followers returned status %d", resp.StatusCode)
	}

	var out helixFollowResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return time.Time{}, false, err
	}
	if len(out.Data) == 0 {
		return time.Time{}, false, nil // not following
	}

	followedAt, err := time.Parse(time.RFC3339, out.Data[0].FollowedAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse followed_at: %w", err)
	}
	return followedAt, true, nil
}

// handleFollowageCommand implements "!followage" (optionally "!followage <user>").
// Everyone may use it. Without an argument it reports the caller's own followage.
func (c *ChatClient) handleFollowageCommand(message gotwitch.PrivateMessage) {
	// Cooldown, reusing the shared per-command cooldown map.
	lookup := c.lookupCommand(BuiltinFollowage)
	if lookup.ok && time.Since(lookup.lastUsed) < c.cooldownDuration {
		return
	}
	c.mu.Lock()
	c.lastUsage[BuiltinFollowage] = time.Now()
	c.mu.Unlock()

	// Determine the target: argument if present, else the caller.
	targetLogin := message.User.Name
	targetDisplay := message.User.DisplayName
	if targetDisplay == "" {
		targetDisplay = message.User.Name
	}
	if _, arg, found := strings.Cut(strings.TrimSpace(message.Message), " "); found {
		arg = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(arg), "@"))
		if arg != "" {
			targetLogin = arg
			targetDisplay = arg
		}
	}

	go func() {
		var targetID string
		var err error
		// If the caller queried themselves, we already have the ID on the message.
		if strings.EqualFold(targetLogin, message.User.Name) && message.User.ID != "" {
			targetID = message.User.ID
		} else {
			targetID, err = c.resolveUserIDByLogin(targetLogin)
			if err != nil {
				c.logger.Warn("followage: could not resolve user", zap.String("login", targetLogin), zap.Error(err))
				c.say(message.Channel, fmt.Sprintf("Could not find Twitch user %q.", targetDisplay))
				return
			}
		}

		followedAt, following, err := c.getFollowedAt(targetID)
		if err != nil {
			c.logger.Error("followage lookup failed", zap.String("user", targetLogin), zap.Error(err))
			c.say(message.Channel, "Sorry, I couldn't check followage right now.")
			return
		}
		if !following {
			c.say(message.Channel, fmt.Sprintf("%s is not following the channel.", targetDisplay))
			return
		}

		dur := time.Since(followedAt)
		c.say(message.Channel, fmt.Sprintf("%s has been following for %s (since %s).",
			targetDisplay, humanizeDuration(dur), followedAt.Format("2006-01-02")))
	}()
}

// say is a small rate-limited wrapper around the IRC client's Say.
func (c *ChatClient) say(channel, text string) {
	if err := c.sayLimiter.Wait(context.Background()); err != nil {
		c.logger.Warn("say rate limit exceeded", zap.Error(err))
		return
	}
	c.client.Say(channel, text)
}

// humanizeDuration renders a duration as "X years, Y months, Z days" (approx),
// falling back to hours/minutes for short spans.
func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	totalDays := int(d.Hours()) / 24
	years := totalDays / 365
	rem := totalDays % 365
	months := rem / 30
	days := rem % 30

	var parts []string
	if years > 0 {
		parts = append(parts, plural(years, "year"))
	}
	if months > 0 {
		parts = append(parts, plural(months, "month"))
	}
	if days > 0 && years == 0 { // hide days once we're into years, keeps it short
		parts = append(parts, plural(days, "day"))
	}
	if len(parts) == 0 {
		hours := int(d.Hours())
		if hours > 0 {
			return plural(hours, "hour")
		}
		return plural(int(d.Minutes()), "minute")
	}
	return strings.Join(parts, ", ")
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
