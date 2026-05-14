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
	bot        *bot.DiscordBot
	mixer      *stream.Mixer
	srt        *stream.SRTManager
	audioPipe  *internal_audio.Pipe
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

	m.mixer = stream.NewMixer()
	if err := m.mixer.Start(); err != nil {
		log.Printf("[AudioBridge] Mixer start error: %v", err)
	}

	m.srt = stream.NewSRTManager()
	if err := m.srt.Start(); err != nil {
		log.Printf("[AudioBridge] SRT manager start error: %v", err)
	}

	m.audioPipe = internal_audio.NewPipe()
	if err := m.audioPipe.Start(); err != nil {
		log.Printf("[AudioBridge] Internal audio pipe start error: %v", err)
	}

	m.bot = bot.NewBot()
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
