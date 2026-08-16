package module

import (
	"fmt"
	"log"
	"sync"
)

// moduleState tracks the runtime state of a single registered module.
type moduleState struct {
	running bool
	lastErr string
}

// ModuleStatus is an exported, lock-free snapshot of a module's registration
// and run state, suitable for serialization by the control API.
type ModuleStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	LastErr string `json:"last_err,omitempty"`
}

// Manager handles the lifecycle of modules.
type Manager struct {
	modules map[string]Module
	states  map[string]*moduleState
	mu      sync.Mutex
}

// NewManager creates a new ModuleManager.
func NewManager() *Manager {
	return &Manager{
		modules: make(map[string]Module),
		states:  make(map[string]*moduleState),
	}
}

// Register adds a module to the manager. A module is registered in the stopped
// state; whether it is started is decided separately (at boot from the settings
// file, or at runtime via the control API).
func (m *Manager) Register(mod Module) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modules[mod.Name()] = mod
	if _, ok := m.states[mod.Name()]; !ok {
		m.states[mod.Name()] = &moduleState{}
	}
}

// StartAll starts all registered modules. It stops at the first failure,
// preserving the original fail-fast behavior. Modules that started successfully
// before the failure are left running.
func (m *Manager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, mod := range m.modules {
		log.Printf("Starting module: %s...", name)
		if err := mod.Start(); err != nil {
			m.setState(name, false, err)
			return fmt.Errorf("failed to start module %s: %w", name, err)
		}
		m.setState(name, true, nil)
	}
	return nil
}

// StopAll stops all registered modules that are currently running. Modules that
// were never started are skipped, so registering every module up front (to make
// runtime enablement possible) does not cause Stop() to run against
// uninitialized module state during shutdown.
func (m *Manager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var stopErrors []error
	for name, mod := range m.modules {
		st := m.states[name]
		if st == nil || !st.running {
			continue
		}
		log.Printf("Stopping module: %s...", name)
		if err := mod.Stop(); err != nil {
			stopErrors = append(stopErrors, fmt.Errorf("failed to stop module %s: %w", name, err))
			m.setState(name, false, err)
			continue
		}
		m.setState(name, false, nil)
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
		m.setState(name, false, err)
		return fmt.Errorf("failed to start module %s: %w", name, err)
	}
	m.setState(name, true, nil)
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

	st := m.states[name]
	if st == nil || !st.running {
		return nil
	}

	log.Printf("Stopping module: %s...", name)
	if err := mod.Stop(); err != nil {
		m.setState(name, false, err)
		return fmt.Errorf("failed to stop module %s: %w", name, err)
	}
	m.setState(name, false, nil)
	return nil
}

// IsRunning reports whether a named module is currently running.
func (m *Manager) IsRunning(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.states[name]; ok {
		return st.running
	}
	return false
}

// Statuses returns a snapshot of every registered module's run state.
func (m *Manager) Statuses() []ModuleStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]ModuleStatus, 0, len(m.modules))
	for name := range m.modules {
		ms := ModuleStatus{Name: name}
		if st := m.states[name]; st != nil {
			ms.Running = st.running
			ms.LastErr = st.lastErr
		}
		out = append(out, ms)
	}
	return out
}

// setState updates the tracked state for a module. Callers must hold m.mu.
func (m *Manager) setState(name string, running bool, err error) {
	st, ok := m.states[name]
	if !ok {
		st = &moduleState{}
		m.states[name] = st
	}
	st.running = running
	if err != nil {
		st.lastErr = err.Error()
	} else {
		st.lastErr = ""
	}
}
