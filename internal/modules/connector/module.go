package connector

import (
	"encoding/json"
	"log"
	"net"
	"strings"
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
	stopChan       chan struct{}
}

// NewModule creates a new instance of the Connector module.
func NewModule(cfg *config.Config, ctrl module.Controller) *Module {
	return &Module{
		config:     cfg,
		controller: ctrl,
		stopChan:   make(chan struct{}),
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

	if m.config.Connector.IPCControlOut {
		log.Println("[Connector] Control IPC Out is ENABLED")
		go m.controlWriterLoop()
	}

	log.Println("[Connector] Started successfully.")
	return nil
}

// Stop cleanly shuts down the Connector components.
func (m *Module) Stop() error {
	log.Println("[Connector] Stopping module...")
	close(m.stopChan)

	log.Println("[Connector] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "Connector"
}

func (m *Module) controlWriterLoop() {
	var conn net.Conn
	var err error
	socketPath := m.config.Connector.ControlSocket
	if socketPath == "" {
		socketPath = "/tmp/vlx_control.sock"
	}

	for {
		select {
		case <-m.stopChan:
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

			// NEW BLOCK: Intercept legacy [ZMQ_CONTROL] text commands
			textVal, hasText := innerPayload["text"].(string)
			if hasText && strings.HasPrefix(strings.TrimSpace(textVal), "[ZMQ_CONTROL]") {
				lines := strings.Split(textVal, "\n")
				var target string
				var enabled bool
				action := "set_input_state" // default
				var path string

				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "Target=") {
						target = strings.TrimPrefix(line, "Target=")
					} else if strings.HasPrefix(line, "Enabled=") {
						val := strings.ToLower(strings.TrimPrefix(line, "Enabled="))
						enabled = (val == "true")
					} else if strings.HasPrefix(line, "Action=") {
						action = strings.TrimPrefix(line, "Action=")
					} else if strings.HasPrefix(line, "Path=") {
						path = strings.TrimPrefix(line, "Path=")
					}
				}

				if target != "" {
					parsedEvent := ConnectorPayload{
						EventID:   uuid.New().String(),
						Timestamp: time.Now().Unix(),
						Action:    action,
						Target:    target,
					}
					if action == "set_input_state" {
						if path != "" {
							parsedEvent.Payload = map[string]interface{}{"enabled": enabled, "text": path}
						} else {
							parsedEvent.Payload = map[string]interface{}{"enabled": enabled}
						}
					} else {
						parsedEvent.Payload = map[string]interface{}{}
					}
					eventsToSend = append(eventsToSend, parsedEvent)
				}
			} else if eventType == "ipc_control" {
				// Parse dynamic IPC payload from ChatFlow
				action, _ := innerPayload["action"].(string)
				if action == "" {
					action = "set_input_state"
				}
				target, _ := innerPayload["target"].(string)
				enabled, _ := innerPayload["enabled"].(bool)
				path, _ := innerPayload["path"].(string)

				connectorEvent := ConnectorPayload{
					EventID:   uuid.New().String(),
					Timestamp: time.Now().Unix(),
					Action:    action,
					Target:    target,
				}
				if action == "set_input_state" {
					if path != "" {
						connectorEvent.Payload = map[string]interface{}{"enabled": enabled, "text": path}
					} else {
						connectorEvent.Payload = map[string]interface{}{"enabled": enabled}
					}
				} else {
					connectorEvent.Payload = map[string]interface{}{}
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
