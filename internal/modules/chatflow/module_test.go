package chatflow

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/module"
)

func TestHandleModuleToggle(t *testing.T) {
	manager := module.NewManager()
	cfg := &config.Config{}

	cfModule := NewModule(cfg, manager, http.NewServeMux())
	manager.Register(cfModule)

	// Test starting a module
	req, err := http.NewRequest("POST", "/api/modules/TestModule/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(cfModule.handleModuleToggle)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for start: got %v want %v", status, http.StatusOK)
	}

	if !bytes.Contains(rr.Body.Bytes(), []byte("Initiated start for module TestModule")) {
		t.Errorf("handler returned unexpected body for start: got %v", rr.Body.String())
	}

	// Test stopping a module
	reqStop, err := http.NewRequest("POST", "/api/modules/TestModule/stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rrStop := httptest.NewRecorder()
	handler.ServeHTTP(rrStop, reqStop)

	if status := rrStop.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code for stop: got %v want %v", status, http.StatusOK)
	}

	if !bytes.Contains(rrStop.Body.Bytes(), []byte("Initiated stop for module TestModule")) {
		t.Errorf("handler returned unexpected body for stop: got %v", rrStop.Body.String())
	}

	// Test invalid method
	reqGet, err := http.NewRequest("GET", "/api/modules/TestModule/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rrGet := httptest.NewRecorder()
	handler.ServeHTTP(rrGet, reqGet)

	if status := rrGet.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler returned wrong status code for GET method: got %v want %v", status, http.StatusMethodNotAllowed)
	}

	// Test invalid action
	reqInvalid, err := http.NewRequest("POST", "/api/modules/TestModule/invalid", nil)
	if err != nil {
		t.Fatal(err)
	}

	rrInvalid := httptest.NewRecorder()
	handler.ServeHTTP(rrInvalid, reqInvalid)

	if status := rrInvalid.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code for invalid action: got %v want %v", status, http.StatusBadRequest)
	}
}
