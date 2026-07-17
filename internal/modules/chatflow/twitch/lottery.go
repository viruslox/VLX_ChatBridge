package twitch

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gotwitch "github.com/gempir/go-twitch-irc/v4"
	"go.uber.org/zap"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Built-in lottery command name (shown in !commands).
const BuiltinLottery = "lottery"

// Default watch window if a lottery is started without an explicit value.
const defaultLotteryWatchWindow = 10 * time.Minute

// LotteryConfig holds the parameters for a lottery round. It can be sourced
// either from an owner file-config (owner_lottery.txt) or from chat arguments.
type LotteryConfig struct {
	// WatchWindow is how recently a joining user must have been active in chat
	// to be considered "watching the live". If 0, presence is not required.
	WatchWindow time.Duration
	// JoinKeyword is the sub-command users type to enter, e.g. "join".
	JoinKeyword string
}

// entrant records a user who successfully joined the current lottery.
type entrant struct {
	Login   string
	Display string
}

// Lottery is a single-round drawing. Only one round is active at a time.
// The zero value is not usable; construct via newLottery.
type Lottery struct {
	mu       sync.Mutex
	active   bool
	cfg      LotteryConfig
	entrants map[string]entrant // normalized login -> entrant
	winner   *entrant
}

func newLottery() *Lottery {
	return &Lottery{
		entrants: make(map[string]entrant),
	}
}

// LotteryFileConfigName is the owner file that (when present) supplies default
// lottery parameters. Placed in static/chat/owner/ like other owner commands.
const LotteryFileConfigName = "owner_lottery.txt"

// loadLotteryFileConfig reads owner_lottery.txt for default parameters. The
// format is simple key=value lines, e.g.:
//
//	WatchWindow=10m
//	JoinKeyword=join
//
// Missing file or fields fall back to defaults. This is the "start textfile
// command" surface: editing this file configures how the watch-check behaves.
func (c *ChatClient) loadLotteryFileConfig() LotteryConfig {
	cfg := LotteryConfig{
		WatchWindow: defaultLotteryWatchWindow,
		JoinKeyword: "join",
	}

	path := filepath.Join(c.config.ChatBridgeDIR, "static", "chat", "owner", LotteryFileConfigName)
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg // file optional
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		val = strings.TrimSpace(val)
		switch key {
		case "watchwindow", "watch_window":
			if d, err := time.ParseDuration(val); err == nil {
				cfg.WatchWindow = d
			} else if secs, err2 := strconv.Atoi(val); err2 == nil {
				cfg.WatchWindow = time.Duration(secs) * time.Second
			}
		case "joinkeyword", "join_keyword":
			if val != "" {
				cfg.JoinKeyword = strings.ToLower(val)
			}
		}
	}
	return cfg
}

// handleLotteryCommand routes "!lottery ..." sub-commands.
//
// Owner-only:
//
//	!lottery start [window]   e.g. !lottery start 15m   (window overrides file config)
//	!lottery draw             pick a winner from current entrants
//	!lottery end              close the round without drawing (or after drawing)
//
// Everyone:
//
//	!lottery join             enter the current round (subject to watch-check)
func (c *ChatClient) handleLotteryCommand(message gotwitch.PrivateMessage) {
	_, args, _ := strings.Cut(strings.TrimSpace(message.Message), " ")
	sub, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	sub = strings.ToLower(strings.TrimSpace(sub))
	rest = strings.TrimSpace(rest)

	isOwner := c.hasPermission(message.User, PermissionOwner)

	switch sub {
	case "start":
		if !isOwner {
			return
		}
		c.lotteryStart(message, rest)
	case "draw", "extract", "pick":
		if !isOwner {
			return
		}
		c.lotteryDraw(message)
	case "end", "finish", "stop":
		if !isOwner {
			return
		}
		c.lotteryEnd(message)
	case "", "join", "enter":
		// Bare "!lottery" and "!lottery join" both mean "enter".
		c.lotteryJoin(message)
	default:
		// Unknown sub-command: treat as a join attempt to be forgiving.
		c.lotteryJoin(message)
	}
}

func (c *ChatClient) lotteryStart(message gotwitch.PrivateMessage, rest string) {
	cfg := c.loadLotteryFileConfig()

	// Optional inline override of the watch window: "!lottery start 15m" or "... 900".
	if rest != "" {
		if d, err := time.ParseDuration(rest); err == nil {
			cfg.WatchWindow = d
		} else if secs, err2 := strconv.Atoi(rest); err2 == nil {
			cfg.WatchWindow = time.Duration(secs) * time.Second
		}
	}

	c.lottery.mu.Lock()
	c.lottery.active = true
	c.lottery.cfg = cfg
	c.lottery.entrants = make(map[string]entrant)
	c.lottery.winner = nil
	c.lottery.mu.Unlock()

	windowMsg := "no watch requirement"
	if cfg.WatchWindow > 0 {
		windowMsg = "watching for at least the last " + humanizeShort(cfg.WatchWindow)
	}
	c.say(message.Channel, fmt.Sprintf(
		"🎉 A lottery has started! Type !lottery %s to enter (%s).",
		cfg.JoinKeyword, windowMsg))
	c.logger.Info("Lottery started",
		zap.Duration("watch_window", cfg.WatchWindow),
		zap.String("join_keyword", cfg.JoinKeyword))
}

func (c *ChatClient) lotteryJoin(message gotwitch.PrivateMessage) {
	c.lottery.mu.Lock()
	active := c.lottery.active
	window := c.lottery.cfg.WatchWindow
	c.lottery.mu.Unlock()

	if !active {
		return // silently ignore joins when no round is open
	}

	login := normalizeUser(message.User.Name)
	display := message.User.DisplayName
	if display == "" {
		display = message.User.Name
	}

	// Watch-check: the joining user must have recent chat activity. Their own
	// join message already counts as activity, so we record it first, then
	// evaluate the window. This means "typing !lottery join" inherently proves
	// live presence; the window matters when re-validating at draw time.
	if c.presence != nil {
		c.presence.Touch(message.User.Name)
		if !c.presence.IsWatching(message.User.Name, window) {
			c.say(message.Channel, fmt.Sprintf(
				"@%s you were excluded from the lottery: you don't appear to be watching the live.",
				display))
			return
		}
	}

	c.lottery.mu.Lock()
	if _, exists := c.lottery.entrants[login]; exists {
		c.lottery.mu.Unlock()
		return // already entered, no spam
	}
	c.lottery.entrants[login] = entrant{Login: message.User.Name, Display: display}
	count := len(c.lottery.entrants)
	c.lottery.mu.Unlock()

	c.logger.Info("Lottery join", zap.String("user", login), zap.Int("total", count))
}

func (c *ChatClient) lotteryDraw(message gotwitch.PrivateMessage) {
	c.lottery.mu.Lock()
	if !c.lottery.active {
		c.lottery.mu.Unlock()
		c.say(message.Channel, "There is no active lottery to draw from.")
		return
	}
	window := c.lottery.cfg.WatchWindow

	// Re-validate presence at draw time: entrants who stopped watching are
	// dropped and notified. This enforces "watching for a while", not just at
	// the moment of joining.
	eligible := make([]entrant, 0, len(c.lottery.entrants))
	var excluded []entrant
	for login, e := range c.lottery.entrants {
		if c.presence == nil || c.presence.IsWatching(login, window) {
			eligible = append(eligible, e)
		} else {
			excluded = append(excluded, e)
			delete(c.lottery.entrants, login)
		}
	}

	if len(eligible) == 0 {
		c.lottery.mu.Unlock()
		c.say(message.Channel, "No eligible entrants are currently watching — no winner drawn.")
		c.notifyExcluded(message.Channel, excluded)
		return
	}

	win := eligible[rand.Intn(len(eligible))]
	c.lottery.winner = &win
	c.lottery.mu.Unlock()

	c.notifyExcluded(message.Channel, excluded)
	c.say(message.Channel, fmt.Sprintf("🏆 The lottery winner is @%s! Congratulations!", win.Display))
	c.logger.Info("Lottery winner drawn", zap.String("winner", win.Login))
}

func (c *ChatClient) lotteryEnd(message gotwitch.PrivateMessage) {
	c.lottery.mu.Lock()
	if !c.lottery.active {
		c.lottery.mu.Unlock()
		c.say(message.Channel, "There is no active lottery to end.")
		return
	}
	c.lottery.active = false
	winner := c.lottery.winner
	entrantCount := len(c.lottery.entrants)
	c.lottery.entrants = make(map[string]entrant)
	c.lottery.winner = nil
	c.lottery.mu.Unlock()

	if winner != nil {
		c.say(message.Channel, fmt.Sprintf("The lottery is closed. Final winner: @%s. Thanks to all %d entrants!",
			winner.Display, entrantCount))
	} else {
		c.say(message.Channel, fmt.Sprintf("The lottery is closed with %d entrants. No winner was drawn.", entrantCount))
	}
	c.logger.Info("Lottery ended", zap.Int("entrants", entrantCount))
}

// notifyExcluded messages users who were dropped for no longer watching.
func (c *ChatClient) notifyExcluded(channel string, excluded []entrant) {
	if len(excluded) == 0 {
		return
	}
	names := make([]string, 0, len(excluded))
	for _, e := range excluded {
		names = append(names, "@"+e.Display)
	}
	c.say(channel, fmt.Sprintf(
		"The following users were excluded (no longer watching the live): %s",
		strings.Join(names, ", ")))
}

// humanizeShort renders a duration compactly (e.g. "10m", "1h30m").
func humanizeShort(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}
