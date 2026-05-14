package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge"
	"VLX_ChatBridge/internal/modules/chatflow"
)

func main() {
	configPath := flag.String("config", "config.example.yml", "Path to configuration file")
	flag.Parse()

	log.Printf("Starting VLX_ChatBridge...")
	log.Printf("Loading configuration from %s", *configPath)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize Module Manager
	manager := module.NewManager()

	if cfg.Modules.ChatFlowEnabled {
		log.Println("ChatFlow module is ENABLED. Registering ChatFlow module...")
		cfModule := chatflow.NewModule(cfg)
		manager.Register(cfModule)
	} else {
		log.Println("ChatFlow module is DISABLED.")
	}

	if cfg.Modules.AudioBridgeEnabled {
		log.Println("AudioBridge module is ENABLED. Registering AudioBridge module...")
		abModule := audiobridge.NewModule(cfg)
		manager.Register(abModule)
	} else {
		log.Println("AudioBridge module is DISABLED.")
	}

	// Start all registered modules
	if err := manager.StartAll(); err != nil {
		log.Fatalf("Failed to start modules: %v", err)
	}

	// Wait for interrupt signal to gracefully shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down VLX_ChatBridge...")

	// Stop modules gracefully
	if err := manager.StopAll(); err != nil {
		log.Printf("Errors during shutdown: %v", err)
	}

	log.Println("Shutdown complete.")
}
