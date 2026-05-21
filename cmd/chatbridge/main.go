package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/install"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge"
	"VLX_ChatBridge/internal/modules/chatflow"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install" {
		install.Run()
		return
	}

	configPath := flag.String("config", "config/chatbridge.settings.template", "Path to configuration file")
	flag.Parse()

	if flag.NArg() > 0 {
		*configPath = flag.Arg(0)
	}

	log.Printf("Starting VLX_ChatBridge...")
	log.Printf("Loading configuration from %s", *configPath)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Println("--- Application Configuration Status ---")
	log.Printf("Module ChatFlow: %v", cfg.Modules.ChatFlowEnabled)
	log.Printf("Module AudioBridge: %v", cfg.Modules.AudioBridgeEnabled)
	log.Printf("Overlay Enable: %v", cfg.Overlay.Enable)
	log.Printf("Overlay Emotes HTML: %v", cfg.Overlay.Emotes.HTML)
	log.Printf("Overlay Emotes Discord: %v", cfg.Overlay.Emotes.Discord)
	log.Printf("Overlay Emotes Streaming: %v", cfg.Overlay.Emotes.Streaming)
	log.Printf("Overlay Alerts HTML: %v", cfg.Overlay.Alerts.HTML)
	log.Printf("Overlay Alerts Discord: %v", cfg.Overlay.Alerts.Discord)
	log.Printf("Overlay Alerts Streaming: %v", cfg.Overlay.Alerts.Streaming)
	log.Printf("Overlay Chat HTML: %v", cfg.Overlay.Chat.HTML)
	log.Printf("Overlay Chat Discord: %v", cfg.Overlay.Chat.Discord)
	log.Printf("Overlay Chat Streaming: %v", cfg.Overlay.Chat.Streaming)
	log.Printf("Discord Streaming (Capture): %v", cfg.Discord.Streaming)
	log.Printf("AudioSource Enable: %v", cfg.AudioSource.Enable)
	log.Printf("AudioSource Discord: %v", cfg.AudioSource.Discord)
	log.Printf("AudioSource Streaming: %v", cfg.AudioSource.Streaming)
	log.Println("----------------------------------------")

	// Initialize Module Manager

	// Initialize Module Manager
	manager := module.NewManager()

	if cfg.Modules.ChatFlowEnabled {
		log.Println("ChatFlow module is ENABLED. Registering ChatFlow module...")
		cfModule := chatflow.NewModule(cfg, manager)
		manager.Register(cfModule)
	} else {
		log.Println("ChatFlow module is DISABLED.")
	}

	if cfg.Modules.AudioBridgeEnabled {
		log.Println("AudioBridge module is ENABLED. Registering AudioBridge module...")
		abModule := audiobridge.NewModule(cfg, manager)
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
