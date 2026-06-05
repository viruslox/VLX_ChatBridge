package websocket

import (
	"encoding/json"

	"VLX_ChatBridge/internal/core/events"
	"go.uber.org/zap"
)

import (
	"sync"
)

// Hub manages the set of active clients and broadcasts messages.
type Hub struct {
	mu         sync.Mutex
	clients    map[*Client]bool
	Broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	logger     *zap.Logger
}

func NewHub(logger *zap.Logger) *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		logger:     logger,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("New WebSocket client registered")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("WebSocket client unregistered")
			}
			h.mu.Unlock()

		case message := <-h.Broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					h.logger.Warn("Client buffer full, forcing unregister")
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()

		case message := <-events.WebSocketBroadcastChan:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					h.logger.Warn("Client buffer full, forcing unregister")
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// BroadcastJSON marshals a payload to JSON and sends it to the Broadcast channel.
func (h *Hub) BroadcastJSON(payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("Failed to marshal payload to JSON", zap.Error(err))
		return err
	}

	select {
	case events.ControlBroadcastChan <- data:
	default:
	}

	h.Broadcast <- data
	return nil
}
