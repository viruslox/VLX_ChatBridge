package audiobridge

import (
	"log"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

// Module represents the AudioBridge component.
type Module struct {
	config     *config.Config
	controller module.Controller
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
	// TODO: Initialize Discord Bot, Mixer, SRT routing
	log.Println("[AudioBridge] Started successfully.")
	return nil
}

// Stop cleanly shuts down the AudioBridge components.
func (m *Module) Stop() error {
	log.Println("[AudioBridge] Stopping module...")
	// TODO: Cleanup Discord connection, shut down mixer, stop SRT stream
	log.Println("[AudioBridge] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "AudioBridge"
}
