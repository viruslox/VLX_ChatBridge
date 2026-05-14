package module

import (
	"fmt"
	"log"
	"sync"
)

// Manager handles the lifecycle of modules.
type Manager struct {
	modules map[string]Module
	mu      sync.Mutex
}

// NewManager creates a new ModuleManager.
func NewManager() *Manager {
	return &Manager{
		modules: make(map[string]Module),
	}
}

// Register adds a module to the manager.
func (m *Manager) Register(mod Module) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modules[mod.Name()] = mod
}

// StartAll starts all registered modules.
func (m *Manager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, mod := range m.modules {
		log.Printf("Starting module: %s...", name)
		if err := mod.Start(); err != nil {
			return fmt.Errorf("failed to start module %s: %w", name, err)
		}
	}
	return nil
}

// StopAll stops all registered modules.
func (m *Manager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var stopErrors []error
	for name, mod := range m.modules {
		log.Printf("Stopping module: %s...", name)
		if err := mod.Stop(); err != nil {
			stopErrors = append(stopErrors, fmt.Errorf("failed to stop module %s: %w", name, err))
		}
	}

	if len(stopErrors) > 0 {
		return fmt.Errorf("errors occurred while stopping modules: %v", stopErrors)
	}
	return nil
}

// StartModule starts a specific module by name.
func (m *Manager) StartModule(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod, exists := m.modules[name]
	if !exists {
		return fmt.Errorf("module %s not found", name)
	}

	log.Printf("Starting module: %s...", name)
	if err := mod.Start(); err != nil {
		return fmt.Errorf("failed to start module %s: %w", name, err)
	}
	return nil
}

// StopModule stops a specific module by name.
func (m *Manager) StopModule(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mod, exists := m.modules[name]
	if !exists {
		return fmt.Errorf("module %s not found", name)
	}

	log.Printf("Stopping module: %s...", name)
	if err := mod.Stop(); err != nil {
		return fmt.Errorf("failed to stop module %s: %w", name, err)
	}
	return nil
}
