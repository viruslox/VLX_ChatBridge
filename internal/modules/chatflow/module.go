package chatflow

import (
	"log"

	"VLX_ChatBridge/internal/core/config"
)

// Module represents the ChatFlow component.
type Module struct {
	config *config.Config
}

// NewModule creates a new instance of the ChatFlow module.
func NewModule(cfg *config.Config) *Module {
	return &Module{
		config: cfg,
	}
}

// Start initializes and starts the ChatFlow components.
func (m *Module) Start() error {
	log.Println("[ChatFlow] Starting module...")
	// TODO: Initialize HTTP server, WebSockets, Twitch, YouTube
	log.Println("[ChatFlow] Started successfully.")
	return nil
}

// Stop cleanly shuts down the ChatFlow components.
func (m *Module) Stop() error {
	log.Println("[ChatFlow] Stopping module...")
	// TODO: Cleanup resources, shutdown server
	log.Println("[ChatFlow] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "ChatFlow"
}
