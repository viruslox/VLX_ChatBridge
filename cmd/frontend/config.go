package main

import (
	"bufio"
	"log"
	"os"
	"strings"
)

type frontendConfig struct {
	BindAddr    string
	BindPort    string
	GUIUser     string
	GUIPass     string
	BackendAddr string
	BackendPort string
	BackendUser string
	BackendPass string
}

// loadFrontendConfig parses a simple key=value settings file. Unknown keys are
// ignored; missing values fall back to sensible defaults.
func loadFrontendConfig(path string) frontendConfig {
	cfg := frontendConfig{
		BindAddr:    "0.0.0.0",
		BindPort:    "8090",
		BackendAddr: "127.0.0.1",
		BackendPort: "8760",
	}

	f, err := os.Open(path)
	if err != nil {
		log.Printf("frontend: could not open %s (%v); using defaults", path, err)
		return cfg
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "bind_address":
			cfg.BindAddr = v
		case "bind_port":
			cfg.BindPort = v
		case "CB_GUI_USER":
			cfg.GUIUser = v
		case "CB_GUI_PASS":
			cfg.GUIPass = v
		case "backend_address":
			cfg.BackendAddr = v
		case "backend_port":
			cfg.BackendPort = v
		case "backend_user":
			cfg.BackendUser = v
		case "backend_pass":
			cfg.BackendPass = v
		}
	}
	return cfg
}
