package connector

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/events"
	"VLX_ChatBridge/internal/core/module"
)

// Module represents the Connector component for local IPC to VLX_VisionBridge
type Module struct {
	config     *config.Config
	controller module.Controller
	mu         sync.Mutex
	stopChan   chan struct{}
}

// NewModule creates a new instance of the Connector module.
func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
	}
}

// ConnectorPayload represents the JSON payload to send over the control socket
type ConnectorPayload struct {
	EventID   string      `json:"event_id"`
	Timestamp int64       `json:"timestamp"`
	Action    string      `json:"action"`
	Target    string      `json:"target"`
	Payload   interface{} `json:"payload"`
}

// Start initializes and starts the Connector components.
func (m *Module) Start() error {
	log.Println("[Connector] Starting module...")

	// A fresh stop channel per start makes the module safe to enable/disable
	// repeatedly at runtime. The running goroutine captures this channel, so a
	// later Stop cannot race with a subsequent Start.
	m.mu.Lock()
	m.stopChan = make(chan struct{})
	stop := m.stopChan
	m.mu.Unlock()

	if m.config.Connector.IPCControlOut {
		log.Println("[Connector] Control IPC Out is ENABLED")
		go m.controlWriterLoop(stop)
	}

	log.Println("[Connector] Started successfully.")
	return nil
}

// Stop cleanly shuts down the Connector components.
func (m *Module) Stop() error {
	log.Println("[Connector] Stopping module...")

	m.mu.Lock()
	if m.stopChan != nil {
		close(m.stopChan)
		m.stopChan = nil
	}
	m.mu.Unlock()

	log.Println("[Connector] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "Connector"
}

func (m *Module) controlWriterLoop(stop <-chan struct{}) {
	var conn net.Conn
	var err error
	socketPath := m.config.Connector.ControlSocket
	if socketPath == "" {
		socketPath = "/tmp/vlx_control.sock"
	}

	for {
		select {
		case <-stop:
			if conn != nil {
				conn.Close()
			}
			return
		case rawPayload := <-events.ControlBroadcastChan:
			if conn == nil {
				conn, err = net.Dial("unix", socketPath)
				if err != nil {
					// VLX_VisionBridge might be down, drop event.
					continue
				}
			}

			// We need to parse raw payload from chatflow and map it to ConnectorPayload
			// If it's just arbitrary json object from BroadcastJSON, we can wrap it.
			var innerPayload map[string]interface{}
			if err := json.Unmarshal(rawPayload, &innerPayload); err != nil {
				continue
			}

			// Determine action/target based on chatflow payload.
			// Currently, chatflow sends "type" field (e.g. "sound_command", "alert", "emote_wall").
			eventType, _ := innerPayload["type"].(string)

			var eventsToSend []ConnectorPayload

			if eventType == "ipc_control" {
				// Parse dynamic IPC payload from ChatFlow
				action, _ := innerPayload["action"].(string)
				if action == "" {
					action = "set_input_state"
				}
				target, _ := innerPayload["target"].(string)
				payload, _ := innerPayload["payload"]

				connectorEvent := ConnectorPayload{
					EventID:   uuid.New().String(),
					Timestamp: time.Now().Unix(),
					Action:    action,
					Target:    target,
					Payload:   payload,
				}
				eventsToSend = append(eventsToSend, connectorEvent)
			} else {
				// For non-control events, just pass through
				connectorEvent := ConnectorPayload{
					EventID:   uuid.New().String(),
					Timestamp: time.Now().Unix(),
					Action:    "trigger_event",
					Target:    eventType,
					Payload:   innerPayload,
				}
				eventsToSend = append(eventsToSend, connectorEvent)
			}

			for _, ev := range eventsToSend {
				outData, err := json.Marshal(ev)
				if err != nil {
					continue
				}

				// Write to control socket, newline delimited json
				outData = append(outData, '\n')

				conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
				_, err = conn.Write(outData)
				if err != nil {
					// Reconnect next time
					conn.Close()
					conn = nil
					break // Break out of sending loop if connection drops
				}
			}
		}
	}
}
