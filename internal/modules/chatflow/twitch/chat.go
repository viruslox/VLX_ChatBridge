package twitch

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/modules/chatflow/websocket"

	"github.com/gempir/go-twitch-irc/v4"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Permission constants
const (
	PermissionEveryone   = "everyone"   // Public/Followers
	PermissionSubscriber = "subscriber" // Paid Subscribers
	PermissionVIP        = "vip"        // VIP/Mods
)

// CommandData holds metadata for media commands
type CommandData struct {
	Filename   string
	Permission string
	MediaType  string // "audio" or "video"
}

type AudioCommandsMap map[string]CommandData

// AnnouncementData holds metadata and content for text announcements
type AnnouncementData struct {
	CommandName string
	Interval    int // in minutes
	Content     string
}

type AnnouncementsMap map[string]AnnouncementData

// ChatClient handles Twitch IRC connection
type ChatClient struct {
	mu               sync.RWMutex
	config           *config.Config
	hub              *websocket.Hub
	client           *twitch.Client
	commands         AudioCommandsMap
	announcements    AnnouncementsMap
	cachedCmdList    string
	lastUsage        map[string]time.Time // Tracks command cooldowns
	cooldownDuration time.Duration        // Configured cooldown
	logger           *zap.Logger
	sayLimiter       *rate.Limiter // Rate limiter for outgoing chat messages
	quit             chan struct{}
}

// UpdateCommands safely updates the command and announcement maps at runtime
func (c *ChatClient) UpdateCommands(cmds AudioCommandsMap, anns AnnouncementsMap) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.commands = cmds
	c.announcements = anns
	c.cachedCmdList = c.formatCommandList()
	c.logger.Info("ChatClient commands and announcements updated via hot reload")
}

// ChatAlertPayload defines the JSON sent to the overlay
type ChatAlertPayload struct {
	Type      string `json:"type"`
	Filename  string `json:"filename"`
	MediaType string `json:"media_type"`
}

type EmoteWallPayload struct {
	Type   string   `json:"type"`
	Emotes []string `json:"emotes"`
}

// NewChatClient initializes the ChatClient with dependencies and rate limiters.
func NewChatClient(cfg *config.Config, hub *websocket.Hub, commands AudioCommandsMap, announcements AnnouncementsMap, logger *zap.Logger) *ChatClient {
	// Set default cooldown if invalid
	cd := cfg.Twitch.Chat.CommandCooldown
	if cd <= 0 {
		cd = 15
	}

	// Initialize Rate Limiter for outgoing messages.
	// Twitch limits: 20/30s for users, 100/30s for mods.
	// We use a conservative bucket: 1 message per second, burst of 5.
	limiter := rate.NewLimiter(rate.Every(time.Second), 5)

	client := &ChatClient{
		config:           cfg,
		hub:              hub,
		commands:         commands,
		announcements:    announcements,
		lastUsage:        make(map[string]time.Time),
		cooldownDuration: time.Duration(cd) * time.Second,
		logger:           logger,
		sayLimiter:       limiter,
		quit:             make(chan struct{}),
	}
	client.cachedCmdList = client.formatCommandList()
	return client
}

// scanCommandFolder processes a single permission folder and adds valid media commands to the map.
func scanCommandFolder(baseDir, folderName, permission string, commands AudioCommandsMap, logger *zap.Logger) {
	fullPath := filepath.Join(baseDir, folderName)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return
	}

	files, err := os.ReadDir(fullPath)
	if err != nil {
		logger.Warn("Could not read command folder", zap.String("path", fullPath), zap.Error(err))
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		ext := strings.ToLower(filepath.Ext(filename))
		commandName := strings.ToLower(strings.TrimSuffix(filename, ext))

		var mediaType string
		switch ext {
		case ".mp3", ".wav", ".ogg":
			mediaType = "audio"
		case ".mp4", ".webm":
			mediaType = "video"
		default:
			continue
		}

		relativePath := folderName + "/" + filename

		if _, exists := commands[commandName]; exists {
			logger.Warn("Duplicate command detected, skipping", zap.String("command", commandName), zap.String("path", relativePath))
		} else {
			commands[commandName] = CommandData{
				Filename:   relativePath,
				Permission: permission,
				MediaType:  mediaType,
			}
		}
	}
}

// ScanAnnouncements scans the announcements folder and parses .txt files.
func ScanAnnouncements(baseDir string, logger *zap.Logger) (AnnouncementsMap, error) {
	fullPath := filepath.Join(baseDir, "announcements")

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, nil // Gracefully handle if directory doesn't exist
	}

	files, err := os.ReadDir(fullPath)
	if err != nil {
		logger.Warn("Could not read announcements folder", zap.String("path", fullPath), zap.Error(err))
		return nil, err
	}

	announcements := make(AnnouncementsMap)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		if strings.ToLower(filepath.Ext(filename)) != ".txt" {
			continue
		}

		baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
		parts := strings.Split(baseName, "_")
		if len(parts) != 2 {
			logger.Warn("Invalid announcement filename format, expected <command>_<interval>.txt", zap.String("filename", filename))
			continue
		}

		commandName := strings.ToLower(parts[0])

		interval, err := strconv.Atoi(parts[1])
		if err != nil {
			logger.Warn("Invalid interval in announcement filename", zap.String("filename", filename), zap.Error(err))
			continue
		}

		contentBytes, err := os.ReadFile(filepath.Join(fullPath, filename))
		if err != nil {
			logger.Warn("Failed to read announcement file", zap.String("filename", filename), zap.Error(err))
			continue
		}
		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			logger.Warn("Announcement file is empty", zap.String("filename", filename))
			continue
		}

		if _, exists := announcements[commandName]; exists {
			logger.Warn("Duplicate announcement command detected, skipping", zap.String("command", commandName), zap.String("filename", filename))
		} else {
			announcements[commandName] = AnnouncementData{
				CommandName: commandName,
				Interval:    interval,
				Content:     content,
			}
		}
	}

	return announcements, nil
}

// ScanAudioCommands recursively scans command folders to build the command map.
func ScanAudioCommands(baseDir string, logger *zap.Logger) (AudioCommandsMap, error) {
	commands := make(AudioCommandsMap)

	folders := map[string]string{
		"everyone":    PermissionEveryone,
		"subscribers": PermissionSubscriber,
		"vips":        PermissionVIP,
	}

	for folderName, permission := range folders {
		scanCommandFolder(baseDir, folderName, permission, commands, logger)
	}

	return commands, nil
}

// startAnnouncementTimers initializes background tickers for each announcement with an interval > 0.
func (c *ChatClient) startAnnouncementTimers() {
	c.mu.RLock()
	for _, ann := range c.announcements {
		if ann.Interval > 0 {
			go func(announcement AnnouncementData) {
				ticker := time.NewTicker(time.Duration(announcement.Interval) * time.Minute)
				defer ticker.Stop()

				for {
					select {
					case <-c.quit:
						return
					case <-ticker.C:
						if err := c.sayLimiter.Wait(context.Background()); err != nil {
							c.logger.Warn("Rate limit exceeded for automatic announcement", zap.String("command", announcement.CommandName), zap.Error(err))
							continue
						}

						// Re-read announcement content to get hot-reloaded content
						c.mu.RLock()
						latestAnn, exists := c.announcements[announcement.CommandName]
						c.mu.RUnlock()

						if !exists {
							// Announcement was removed during hot reload, stop the ticker
							return
						}


						channelToJoin := c.config.Twitch.Chat.ChannelToJoin


						c.client.Say(channelToJoin, latestAnn.Content)
						c.logger.Info("Sent automatic announcement", zap.String("command", latestAnn.CommandName))
					}
				}
			}(ann)
		}
	}
	c.mu.RUnlock()
}

// Stop gracefully disconnects the Twitch IRC client and stops background timers.
func (c *ChatClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.quit:
		// Already closed
	default:
		close(c.quit)
	}

	if c.client != nil {
		c.client.Disconnect()
	}
}

// Start initiates the Twitch IRC connection.
func (c *ChatClient) Start() {
	c.logger.Info("Connecting to Twitch IRC...")


	botUsername := c.config.Twitch.Chat.BotUsername
	botOAuthToken := c.config.Twitch.Chat.BotToken
	channelToJoin := c.config.Twitch.Chat.ChannelToJoin


	c.client = twitch.NewClient(botUsername, botOAuthToken)
	// Force port 443 (SSL)
	c.client.IrcAddress = "irc.chat.twitch.tv:443"

	c.client.OnPrivateMessage(c.handlePrivateMessage)

	c.client.OnConnect(func() {
		c.logger.Info("Connected to IRC channel", zap.String("channel", channelToJoin))
	})

	c.client.Join(channelToJoin)

	// Background reconnection loop
	go func() {
		timer := time.NewTimer(0)
		<-timer.C // Drain it immediately

		for {
			select {
			case <-c.quit:
				return
			default:
			}

			if err := c.client.Connect(); err != nil {
				c.logger.Error("IRC Connection failed. Retrying in 10s...", zap.Error(err))
				timer.Reset(10 * time.Second)
				select {
				case <-c.quit:
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
	}()

	// Start background timers for announcements
	c.startAnnouncementTimers()
}

func (c *ChatClient) handlePrivateMessage(message twitch.PrivateMessage) {
	c.handleEmoteWall(message)
	c.handleCommand(message)
}

func (c *ChatClient) handleEmoteWall(message twitch.PrivateMessage) {
	// 1. EMOTE WALL (Broadcasts valid emotes to WebSocket)
	if len(message.Emotes) > 0 {
		totalCount := 0
		for _, emote := range message.Emotes {
			totalCount += emote.Count
		}

		emoteURLs := make([]string, 0, totalCount)
		for _, emote := range message.Emotes {
			// Twitch CDN URL format 3.0 (Scale)
			url := "https://static-cdn.jtvnw.net/emoticons/v2/" + emote.ID + "/default/dark/3.0"
			for i := 0; i < emote.Count; i++ {
				emoteURLs = append(emoteURLs, url)
			}
		}

		if len(emoteURLs) > 0 {
			payload := EmoteWallPayload{
				Type:   "emote_wall",
				Emotes: emoteURLs,
			}


			htmlEnabled := c.config.Overlay.Enable


			if htmlEnabled {
				c.hub.BroadcastJSON(payload)
			}
		}
	}
}

type commandLookupResult struct {
	annData  AnnouncementData
	isAnn    bool
	cmdData  CommandData
	exists   bool
	lastUsed time.Time
	ok       bool
}

func (c *ChatClient) lookupCommand(commandName string) commandLookupResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	annData, isAnn := c.announcements[commandName]
	cmdData, exists := c.commands[commandName]
	lastUsed, ok := c.lastUsage[commandName]

	return commandLookupResult{
		annData:  annData,
		isAnn:    isAnn,
		cmdData:  cmdData,
		exists:   exists,
		lastUsed: lastUsed,
		ok:       ok,
	}
}

func (c *ChatClient) handleCommand(message twitch.PrivateMessage) {
	// 2. Check for Command Prefix
	if !strings.HasPrefix(message.Message, "!") {
		return
	}

	commandName, _, _ := strings.Cut(message.Message[1:], " ")
	commandName = strings.ToLower(commandName)

	// 3. LIST COMMANDS Logic (!commands)
	if commandName == "commands" || commandName == "comandi" {
		c.handleListCommands(message.Channel)
		return
	}

	lookup := c.lookupCommand(commandName)

	// 4. ANNOUNCEMENT COMMAND Logic (Manual Trigger)
	if lookup.isAnn {
		c.processAnnouncement(commandName, message, lookup)
		return
	}

	// 5. MEDIA COMMAND Logic
	if lookup.exists {
		c.processMediaCommand(commandName, message, lookup)
	}
}

func (c *ChatClient) processAnnouncement(commandName string, message twitch.PrivateMessage, lookup commandLookupResult) {
	if !c.hasPermission(message.User, PermissionVIP) { // Requires VIP/Mod/Broadcaster
		return
	}

	// Cooldown check for announcements
	if lookup.ok {
		if time.Since(lookup.lastUsed) < c.cooldownDuration {
			c.logger.Info("Announcement on cooldown", zap.String("command", commandName), zap.String("user", message.User.Name))
			return
		}
	}

	c.mu.Lock()
	c.lastUsage[commandName] = time.Now()
	c.mu.Unlock()

	if err := c.sayLimiter.Wait(context.Background()); err != nil {
		c.logger.Warn("Rate limit exceeded for manual announcement", zap.String("command", commandName), zap.Error(err))
		return
	}
	c.client.Say(message.Channel, lookup.annData.Content)
	c.logger.Info("Manual announcement triggered", zap.String("command", commandName), zap.String("user", message.User.Name))
}

func (c *ChatClient) processMediaCommand(commandName string, message twitch.PrivateMessage, lookup commandLookupResult) {
	// Permission check
	if !c.hasPermission(message.User, lookup.cmdData.Permission) {
		return
	}

	// --- COOLDOWN CHECK ---
	if lookup.ok {
		if time.Since(lookup.lastUsed) < c.cooldownDuration {
			c.logger.Info("Command on cooldown", zap.String("command", commandName), zap.String("user", message.User.Name))
			return
		}
	}

	c.mu.Lock()
	c.lastUsage[commandName] = time.Now()
	c.mu.Unlock()
	// ----------------------

	c.logger.Info("Command triggered", zap.String("command", commandName), zap.String("user", message.User.Name))

	payload := ChatAlertPayload{
		Type:      "sound_command",
		Filename:  lookup.cmdData.Filename,
		MediaType: lookup.cmdData.MediaType,
	}


	htmlEnabled := c.config.Overlay.Enable


	if htmlEnabled {
		c.hub.BroadcastJSON(payload)
	}
}

// handleListCommands constructs and sends the list of available commands.
func (c *ChatClient) handleListCommands(channel string) {
	// Check outgoing rate limit before sending
	if err := c.sayLimiter.Wait(context.Background()); err != nil {
		c.logger.Warn("Rate limit exceeded for outgoing message", zap.Error(err))
		return
	}

	c.mu.RLock()
	cmdList := c.cachedCmdList
	c.mu.RUnlock()

	c.client.Say(channel, cmdList)
}

// formatCommandList groups and formats the available commands by permission.
func (c *ChatClient) formatCommandList() string {
	var everyone []string
	var subs []string
	var vips []string

	for name, data := range c.commands {
		cmd := "!" + name
		switch data.Permission {
		case PermissionEveryone:
			everyone = append(everyone, cmd)
		case PermissionSubscriber:
			subs = append(subs, cmd)
		case PermissionVIP:
			vips = append(vips, cmd)
		}
	}

	sort.Strings(everyone)
	sort.Strings(subs)
	sort.Strings(vips)

	var sb strings.Builder

	if len(everyone) > 0 {
		sb.WriteString(strings.Join(everyone, ", "))
	}

	if len(subs) > 0 {
		if sb.Len() > 0 {
			sb.WriteString(" / ")
		}
		sb.WriteString("Subscribers: ")
		sb.WriteString(strings.Join(subs, ", "))
	}

	if len(vips) > 0 {
		if sb.Len() > 0 {
			sb.WriteString(" / ")
		}
		sb.WriteString("Vips: ")
		sb.WriteString(strings.Join(vips, ", "))
	}

	response := sb.String()
	if response == "" {
		response = "No active commands found."
	}
	return response
}

// hasPermission checks Twitch badges against required level
func (c *ChatClient) hasPermission(user twitch.User, requiredLevel string) bool {
	// Broadcasters and Mods have all permissions
	if _, ok := user.Badges["broadcaster"]; ok {
		return true
	}
	if _, ok := user.Badges["moderator"]; ok {
		return true
	}

	switch requiredLevel {
	case PermissionEveryone:
		return true
	case PermissionSubscriber:
		// Checks for sub or founder badge
		_, isSub := user.Badges["subscriber"]
		_, isFounder := user.Badges["founder"]
		return isSub || isFounder
	case PermissionVIP:
		_, isVIP := user.Badges["vip"]
		return isVIP
	default:
		return false
	}
}
