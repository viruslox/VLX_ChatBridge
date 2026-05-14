package module

import (
	"errors"
	"testing"
)

type MockModule struct {
	name      string
	startErr  error
	stopErr   error
	started   bool
	stopped   bool
}

func (m *MockModule) Start() error {
	m.started = true
	return m.startErr
}

func (m *MockModule) Stop() error {
	m.stopped = true
	return m.stopErr
}

func (m *MockModule) Name() string {
	return m.name
}

func TestManager_StartStopAll(t *testing.T) {
	manager := NewManager()

	mod1 := &MockModule{name: "Mod1"}
	mod2 := &MockModule{name: "Mod2"}

	manager.Register(mod1)
	manager.Register(mod2)

	if err := manager.StartAll(); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	if !mod1.started || !mod2.started {
		t.Errorf("Expected all modules to be started")
	}

	if err := manager.StopAll(); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	if !mod1.stopped || !mod2.stopped {
		t.Errorf("Expected all modules to be stopped")
	}
}

func TestManager_StartAllError(t *testing.T) {
	manager := NewManager()

	mod1 := &MockModule{name: "Mod1"}
	mod2 := &MockModule{name: "Mod2", startErr: errors.New("start failed")}

	manager.Register(mod1)
	manager.Register(mod2)

	err := manager.StartAll()
	if err == nil {
		t.Fatal("Expected error from StartAll, got none")
	}
}

func TestManager_StartStopModule(t *testing.T) {
	manager := NewManager()

	mod1 := &MockModule{name: "Mod1"}
	manager.Register(mod1)

	if err := manager.StartModule("Mod1"); err != nil {
		t.Fatalf("StartModule failed: %v", err)
	}
	if !mod1.started {
		t.Errorf("Expected Mod1 to be started")
	}

	if err := manager.StopModule("Mod1"); err != nil {
		t.Fatalf("StopModule failed: %v", err)
	}
	if !mod1.stopped {
		t.Errorf("Expected Mod1 to be stopped")
	}

	if err := manager.StartModule("NonExistent"); err == nil {
		t.Error("Expected error for non-existent module on StartModule")
	}

	if err := manager.StopModule("NonExistent"); err == nil {
		t.Error("Expected error for non-existent module on StopModule")
	}
}
