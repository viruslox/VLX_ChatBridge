package audiobridge

import (
	"log"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge/bot"
)

// Module represents the AudioBridge component.
type Module struct {
	config       *config.Config
	controller   module.Controller
	bot          *bot.DiscordBot
	discordMixer *audio.Mixer
}

// NewModule creates a new instance of the AudioBridge module.
func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
	}
}

// Start initializes and starts the AudioBridge components.
func (m *Module) Start() error {
	log.Println("[AudioBridge] Starting module...")

	discordOutChan := make(chan []byte, 1024)

	m.discordMixer = audio.NewMixer("Discord", 100, false, audio.DiscordChannel, discordOutChan)
	if err := m.discordMixer.Start(); err != nil {
		log.Printf("[AudioBridge] Discord Mixer start error: %v", err)
	}

	discordStreaming := m.config.Discord.Streaming
	m.bot = bot.NewBot(m.config.Discord.Token, m.config.Discord.Prefix, m.config.Discord.Admins, bool(discordStreaming), m.config.Discord.ExcludedUsers, m.controller, discordOutChan)
	if err := m.bot.Connect(); err != nil {
		log.Printf("[AudioBridge] Discord bot connect error: %v", err)
	}

	log.Println("[AudioBridge] Started successfully.")
	return nil
}

// Stop cleanly shuts down the AudioBridge components.
func (m *Module) Stop() error {
	log.Println("[AudioBridge] Stopping module...")

	if m.bot != nil {
		m.bot.Disconnect()
	}

	if m.discordMixer != nil {
		m.discordMixer.Stop()
	}

	log.Println("[AudioBridge] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "AudioBridge"
}
