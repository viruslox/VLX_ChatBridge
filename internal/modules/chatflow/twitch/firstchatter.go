package twitch

import (
	"sync"

	gotwitch "github.com/gempir/go-twitch-irc/v4"
	"go.uber.org/zap"
)

// FirstChatterTracker records which users have already chatted during the
// current live session. It is purely in-memory: the set resets when
// ChatBridge restarts, so every user floats again at the start of a new run.
type FirstChatterTracker struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// NewFirstChatterTracker returns an initialized, empty tracker.
func NewFirstChatterTracker() *FirstChatterTracker {
	return &FirstChatterTracker{
		seen: make(map[string]struct{}),
	}
}

// MarkIfFirst records the user and reports whether this was their first
// appearance this session. Returns true exactly once per username per run.
func (f *FirstChatterTracker) MarkIfFirst(user string) bool {
	key := normalizeUser(user) // shared helper from presence.go
	if key == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.seen[key]; exists {
		return false
	}
	f.seen[key] = struct{}{}
	return true
}

// ChatUsernamePayload is broadcast on the same WebSocket the emote wall uses,
// so the emotes overlay can float a first-time chatter's username. A distinct
// Type keeps it separate from emote spawns.
type ChatUsernamePayload struct {
	Type     string `json:"type"`     // always "chat_username"
	Username string `json:"username"` // display name as shown in chat
	Color    string `json:"color"`    // hex like "#1E90FF"
}

// twitchDefaultColors is Twitch's fixed palette used to assign a color to users
// who have never set one. Order matters: the selection index maps into this slice.
var twitchDefaultColors = []string{
	"#FF0000", // Red
	"#0000FF", // Blue
	"#00FF00", // Green
	"#B22222", // FireBrick
	"#FF7F50", // Coral
	"#9ACD32", // YellowGreen
	"#FF4500", // OrangeRed
	"#2E8B57", // SeaGreen
	"#DAA520", // GoldenRod
	"#D2691E", // Chocolate
	"#5F9EA0", // CadetBlue
	"#1E90FF", // DodgerBlue
	"#FF69B4", // HotPink
	"#8A2BE2", // BlueViolet
	"#00FF7F", // SpringGreen
}

// defaultColorForUser reproduces Twitch's default-color algorithm: pick a
// palette entry from the sum of the first and last character code points of the
// login, modulo the palette size. This matches what viewers see in chat for
// users who haven't run /color.
func defaultColorForUser(login string) string {
	runes := []rune(login)
	if len(runes) == 0 {
		return twitchDefaultColors[0]
	}
	first := int(runes[0])
	last := int(runes[len(runes)-1])
	idx := (first + last) % len(twitchDefaultColors)
	return twitchDefaultColors[idx]
}

// resolveChatColor returns the color to render for a message: the user's own
// chat color when set, otherwise the deterministic Twitch default for their
// login, so the floated username matches their chat appearance.
func resolveChatColor(message gotwitch.PrivateMessage) string {
	if c := message.User.Color; c != "" {
		return c
	}
	return defaultColorForUser(message.User.Name)
}

// handleFirstChatter floats a first-time chatter's username through the emote
// overlay. Called on every incoming message; it is a no-op after the user's
// first message this session. Respects the emotes-overlay master switch.
func (c *ChatClient) handleFirstChatter(message gotwitch.PrivateMessage) {
	if c.firstChatters == nil {
		return
	}
	if !c.firstChatters.MarkIfFirst(message.User.Name) {
		return
	}

	htmlEnabled := bool(c.config.Overlay.Enable) && bool(c.config.Overlay.Emotes.HTML)
	if !htmlEnabled {
		return
	}

	display := message.User.DisplayName
	if display == "" {
		display = message.User.Name
	}

	payload := ChatUsernamePayload{
		Type:     "chat_username",
		Username: display,
		Color:    resolveChatColor(message),
	}
	c.hub.BroadcastJSON(payload)
	c.logger.Info("First chatter of session floated", zap.String("user", message.User.Name))
}
