package audiobridge

import (
	"log"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge/bot"
	"VLX_ChatBridge/internal/modules/audiobridge/internal_audio"
	"VLX_ChatBridge/internal/modules/audiobridge/stream"
)

// Module represents the AudioBridge component.
type Module struct {
	config     *config.Config
	controller module.Controller
	bot         *bot.DiscordBot
	mixer       *stream.Mixer
	srt         *stream.SRTManager
	audioSource *stream.AudioSourceManager
	audioPipe   *internal_audio.Pipe
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

	srtChan := make(chan []byte, 1024)

	m.mixer = stream.NewMixer(m.config, srtChan)
	if err := m.mixer.Start(); err != nil {
		log.Printf("[AudioBridge] Mixer start error: %v", err)
	}

	m.srt = stream.NewSRTManager(m.config, srtChan)
	if err := m.srt.Start(); err != nil {
		log.Printf("[AudioBridge] SRT manager start error: %v", err)
	}

	m.audioSource = stream.NewAudioSourceManager(m.config)
	if err := m.audioSource.Start(); err != nil {
		log.Printf("[AudioBridge] Audio Source manager start error: %v", err)
	}

	m.audioPipe = internal_audio.NewPipe()
	if err := m.audioPipe.Start(); err != nil {
		log.Printf("[AudioBridge] Internal audio pipe start error: %v", err)
	}

	discordStreaming := m.config.Discord.Streaming
	m.bot = bot.NewBot(m.config.Discord.Token, m.config.Discord.Prefix, m.config.Discord.Admins, bool(discordStreaming), m.config.Discord.ExcludedUsers, m.controller)
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

	if m.audioPipe != nil {
		m.audioPipe.Stop()
	}

	if m.srt != nil {
		m.srt.Stop()
	}

	if m.audioSource != nil {
		m.audioSource.Stop()
	}

	if m.mixer != nil {
		m.mixer.Stop()
	}

	log.Println("[AudioBridge] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "AudioBridge"
}
