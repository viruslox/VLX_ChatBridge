package connector

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/google/uuid"

	"VLX_ChatBridge/internal/core/audio"
	"VLX_ChatBridge/internal/core/config"
	"VLX_ChatBridge/internal/core/events"
	"VLX_ChatBridge/internal/core/module"
)

// Module represents the Connector component for local IPC to VLX_VisionBridge
type Module struct {
	config     *config.Config
	controller module.Controller

	connectorMixer *audio.Mixer
	audioOutChan   chan []byte
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

	if m.config.Connector.IPCAudioOut {
		log.Println("[Connector] Audio IPC Out is ENABLED")
		m.audioOutChan = make(chan []byte, 1024)
		// Volume is 100 for IPC out. Similar to SRT but we pipe to UDS.
		m.connectorMixer = audio.NewMixer("Connector", 100, true, audio.ConnectorChannel, m.audioOutChan)
		if err := m.connectorMixer.Start(); err != nil {
			log.Printf("[Connector] Mixer start error: %v", err)
		}

		go m.audioWriterLoop()
	}

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

	if m.connectorMixer != nil {
		m.connectorMixer.Stop()
	}

	log.Println("[Connector] Stopped successfully.")
	return nil
}

// Name returns the module identifier.
func (m *Module) Name() string {
	return "Connector"
}

func (m *Module) audioWriterLoop() {
	var conn net.Conn
	var err error
	socketPath := m.config.Connector.AudioSocket
	if socketPath == "" {
		socketPath = "/tmp/vlx_audio.sock"
	}

	for {
		select {
		case <-m.stopChan:
			if conn != nil {
				conn.Close()
			}
			return
		case data := <-m.audioOutChan:
			if conn == nil {
				conn, err = net.Dial("unix", socketPath)
				if err != nil {
					// Drop packet if not connected. VLX_VisionBridge might be down.
					// Sleep briefly to avoid tight spin-loop on dial errors if bombarded.
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			// Non-blocking write approach (using deadline)
			conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
			_, err = conn.Write(data)
			if err != nil {
				// Broken pipe or timeout, close and reset
				conn.Close()
				conn = nil
			}
		}
	}
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

			connectorEvent := ConnectorPayload{
				EventID:   uuid.New().String(),
				Timestamp: time.Now().Unix(),
				Action:    "trigger_event",
				Target:    eventType,
				Payload:   innerPayload,
			}

			var eventsToSend []ConnectorPayload

			// Intercept specific commands to control VisionBridge
			if eventType == "ipc_control" {
				// Parse dynamic IPC payload from ChatFlow
				target, _ := innerPayload["target"].(string)
				enabled, _ := innerPayload["enabled"].(bool)

				connectorEvent.Action = "set_input_state"
				connectorEvent.Target = target
				connectorEvent.Payload = map[string]interface{}{"enabled": enabled}
				eventsToSend = append(eventsToSend, connectorEvent)
			} else {
				// For non-control events, just pass through as trigger_event
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
