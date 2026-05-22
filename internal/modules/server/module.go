package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

// Module represents the Server component.
type Module struct {
	config     *config.Config
	controller module.Controller
	mux        *http.ServeMux
	server     *http.Server
}

// NewModule creates a new instance of the Server module.
func NewModule(cfg *config.Config, ctrl module.Controller, mux *http.ServeMux) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
		mux:        mux,
	}
}

// Start initializes and starts the Server component.
func (m *Module) Start() error {
	log.Println("[Server] Starting module...")

	port := m.config.Server.Port
	if port == "" {
		port = "8000"
	}

	m.server = &http.Server{
		Addr:    ":" + port,
		Handler: m.mux,
	}

	go func() {
		log.Printf("[Server] HTTP server listening on %s", m.server.Addr)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Server] HTTP server error: %v", err)
		}
	}()

	log.Println("[Server] Started successfully.")
	return nil
}

// Stop cleanly shuts down the Server component.
func (m *Module) Stop() error {
	log.Println("[Server] Stopping module...")

	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			log.Printf("[Server] HTTP server shutdown error: %v", err)
		}
	}

	log.Println("[Server] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "Server"
}
