package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"VLX_ChatBridge/internal/core/config"
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

	if cfg.Modules.ChatFlowEnabled {
		log.Println("ChatFlow module is ENABLED. Starting ChatFlow components...")
		// TODO: Initialize and start ChatFlow
	} else {
		log.Println("ChatFlow module is DISABLED.")
	}

	if cfg.Modules.AudioBridgeEnabled {
		log.Println("AudioBridge module is ENABLED. Starting AudioBridge components...")
		// TODO: Initialize and start AudioBridge
	} else {
		log.Println("AudioBridge module is DISABLED.")
	}

	// Wait for interrupt signal to gracefully shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down VLX_ChatBridge...")
	// TODO: Stop modules gracefully
}
