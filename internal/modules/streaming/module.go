package streaming

import (
	"log"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

type Module struct {
	config      *config.Config
	controller  module.Controller
	srtMixer    *audio.Mixer
	srt         *SRTManager
	audioSource *AudioSourceManager
}

func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
	}
}

func (m *Module) Start() error {
	log.Println("[Streaming] Starting module...")

	srtOutChan := make(chan []byte, 1024)

	m.srtMixer = audio.NewMixer("SRT", m.config.Streaming.Volume, true, audio.SRTChannel, srtOutChan)
	if err := m.srtMixer.Start(); err != nil {
		log.Printf("[Streaming] SRT Mixer start error: %v", err)
	}

	m.srt = NewSRTManager(m.config, srtOutChan)
	if err := m.srt.Start(); err != nil {
		log.Printf("[Streaming] SRT manager start error: %v", err)
	}

	m.audioSource = NewAudioSourceManager(m.config)
	if err := m.audioSource.Start(); err != nil {
		log.Printf("[Streaming] Audio Source manager start error: %v", err)
	}

	log.Println("[Streaming] Started successfully.")
	return nil
}

func (m *Module) Stop() error {
	log.Println("[Streaming] Stopping module...")

	if m.srt != nil {
		m.srt.Stop()
	}

	if m.audioSource != nil {
		m.audioSource.Stop()
	}

	if m.srtMixer != nil {
		m.srtMixer.Stop()
	}

	log.Println("[Streaming] Stopped successfully.")
	return nil
}

func (m *Module) Name() string {
	return "Streaming"
}
