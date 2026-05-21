package audiobridge

import (
	"log"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge/bot"
	"VLX_ChatBridge/internal/modules/audiobridge/internal_audio"
	"VLX_ChatBridge/internal/modules/audiobridge/stream"
)

// Module represents the AudioBridge component.
type Module struct {
	config       *config.Config
	controller   module.Controller
	bot          *bot.DiscordBot
	srtMixer     *stream.Mixer
	discordMixer *stream.Mixer
	srt          *stream.SRTManager
	audioSource  *stream.AudioSourceManager
	audioPipe    *internal_audio.Pipe
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

	srtOutChan := make(chan []byte, 1024)
	discordOutChan := make(chan []byte, 1024)

	srtInChan := make(chan audio.StreamData, 1024)
	discordInChan := make(chan audio.StreamData, 1024)

	m.audioPipe = internal_audio.NewPipe(srtInChan, discordInChan)
	if err := m.audioPipe.Start(); err != nil {
		log.Printf("[AudioBridge] Internal audio pipe start error: %v", err)
	}

	if m.config.Streaming.Enable {
		m.srtMixer = stream.NewMixer("SRT", m.config.Streaming.Volume, true, srtInChan, srtOutChan)
		if err := m.srtMixer.Start(); err != nil {
			log.Printf("[AudioBridge] SRT Mixer start error: %v", err)
		}

		m.srt = stream.NewSRTManager(m.config, srtOutChan)
		if err := m.srt.Start(); err != nil {
			log.Printf("[AudioBridge] SRT manager start error: %v", err)
		}
	} else {
		log.Println("[AudioBridge] Streaming is disabled.")
	}

	m.discordMixer = stream.NewMixer("Discord", 100, false, discordInChan, discordOutChan)
	if err := m.discordMixer.Start(); err != nil {
		log.Printf("[AudioBridge] Discord Mixer start error: %v", err)
	}

	m.audioSource = stream.NewAudioSourceManager(m.config)
	if err := m.audioSource.Start(); err != nil {
		log.Printf("[AudioBridge] Audio Source manager start error: %v", err)
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

	if m.audioPipe != nil {
		m.audioPipe.Stop()
	}

	if m.srt != nil {
		m.srt.Stop()
	}

	if m.audioSource != nil {
		m.audioSource.Stop()
	}

	if m.srtMixer != nil {
		m.srtMixer.Stop()
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
