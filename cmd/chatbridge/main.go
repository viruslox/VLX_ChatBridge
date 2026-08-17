package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/controlapi"
	"VLX_ChatBridge/internal/core/install"
	"VLX_ChatBridge/internal/core/module"
	"VLX_ChatBridge/internal/modules/audiobridge"
	"VLX_ChatBridge/internal/modules/audiosource"
	"VLX_ChatBridge/internal/modules/chatflow"
	"VLX_ChatBridge/internal/modules/connector"
	"VLX_ChatBridge/internal/modules/server"
	"VLX_ChatBridge/internal/modules/streaming"
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
	log.Printf("Module Server: %v", cfg.Modules.ServerEnabled)
	log.Printf("Module Streaming: %v", cfg.Modules.StreamingEnabled)
	log.Printf("Module AudioSource: %v", cfg.Modules.AudioSourceEnabled)
	log.Printf("Module Connector: %v", cfg.Modules.ConnectorEnabled)
	log.Printf("Overlay Enable: %v", cfg.Overlay.Enable)
	log.Printf("Overlay Emotes HTML: %v", cfg.Overlay.Emotes.HTML)
	log.Printf("Overlay Alerts HTML: %v", cfg.Overlay.Alerts.HTML)
	log.Printf("Overlay Alerts Discord: %v", cfg.Overlay.Alerts.Discord)
	log.Printf("Overlay Alerts Streaming: %v", cfg.Overlay.Alerts.Streaming)
	log.Printf("Overlay Chat HTML: %v", cfg.Overlay.Chat.HTML)
	log.Printf("Overlay Chat Discord: %v", cfg.Overlay.Chat.Discord)
	log.Printf("Overlay Chat Streaming: %v", cfg.Overlay.Chat.Streaming)
	log.Printf("Discord Streaming (Capture): %v", cfg.Discord.Streaming)
	log.Printf("Streaming Enable: %v", cfg.Streaming.Enable)
	log.Printf("AudioSource Enable: %v", cfg.AudioSource.Enable)
	log.Printf("AudioSource Discord: %v", cfg.AudioSource.Discord)
	log.Printf("AudioSource Streaming: %v", cfg.AudioSource.Streaming)
	log.Printf("Connector IPC Control Out: %v", cfg.Connector.IPCControlOut)
	log.Printf("Connector Control Socket: %s", cfg.Connector.ControlSocket)
	log.Println("----------------------------------------")

	// Initialize Module Manager
	manager := module.NewManager()

	// Shared HTTP mux for server and chatflow
	mux := http.NewServeMux()

	// Construct and register ALL modules unconditionally. Registering every
	// module (regardless of its enabled flag) is what makes on-the-fly runtime
	// enable/disable possible: a module can only be started later if an instance
	// exists to start. Construction is side-effect free for every module (it
	// only stores references), so registering a disabled module is inert.
	//
	// The settings file remains the master: only the modules it marks enabled
	// are STARTED at boot (see the boot loop below); everything else stays
	// registered-but-stopped until explicitly enabled at runtime, and anything
	// the file marks disabled stays off at boot.
	manager.Register(server.NewModule(cfg, manager, mux))
	manager.Register(chatflow.NewModule(cfg, manager, mux))
	manager.Register(audiobridge.NewModule(cfg, manager))
	manager.Register(streaming.NewModule(cfg, manager))
	manager.Register(audiosource.NewModule(cfg, manager))
	manager.Register(connector.NewModule(cfg, manager))

	// Boot-start only the modules enabled in the settings file, in a
	// deterministic order (Server first so the shared mux is served, matching
	// the previous registration order).
	type bootModule struct {
		name    string
		enabled bool
	}
	bootOrder := []bootModule{
		{"Server", bool(cfg.Modules.ServerEnabled)},
		{"ChatFlow", bool(cfg.Modules.ChatFlowEnabled)},
		{"AudioBridge", bool(cfg.Modules.AudioBridgeEnabled)},
		{"Streaming", bool(cfg.Modules.StreamingEnabled)},
		{"AudioSource", bool(cfg.Modules.AudioSourceEnabled)},
		{"Connector", bool(cfg.Modules.ConnectorEnabled)},
	}
	for _, bm := range bootOrder {
		if !bm.enabled {
			log.Printf("%s module is DISABLED in settings; registered but not started.", bm.name)
			continue
		}
		log.Printf("%s module is ENABLED. Starting...", bm.name)
		if err := manager.StartModule(bm.name); err != nil {
			log.Fatalf("Failed to start module %s: %v", bm.name, err)
		}
	}

	// Start the always-on control/status API used by the web GUI. It runs
	// regardless of which modules are enabled so the GUI can always reach the
	// backend. Toggles it performs are persisted to the settings file and take
	// effect on the next (systemd-relaunched) restart.
	triggerShutdown := func() {
		if p, err := os.FindProcess(os.Getpid()); err == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
	}
	var ctrlAPI *controlapi.Server
	if bool(cfg.ControlAPI.Enable) {
		ctrlAPI = controlapi.New(
			*configPath,
			cfg.ControlAPI.BindAddr,
			cfg.ControlAPI.Port,
			cfg.ControlAPI.User,
			cfg.ControlAPI.Pass,
			cfg.ControlAPI.LogUnit,
			manager,
			triggerShutdown,
		)
		ctrlAPI.Start()
	}

	// Wait for interrupt signal to gracefully shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down VLX_ChatBridge...")

	if ctrlAPI != nil {
		ctrlAPI.Stop()
	}

	// Stop modules gracefully (only running modules are stopped)
	if err := manager.StopAll(); err != nil {
		log.Printf("Errors during shutdown: %v", err)
	}

	log.Println("Shutdown complete.")
}
