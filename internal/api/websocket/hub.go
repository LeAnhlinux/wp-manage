package websocket

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type Message struct {
	Type    string      `json:"type"`
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"`
}

type Client struct {
	Conn     *websocket.Conn
	Channels map[string]bool
	mu       sync.Mutex
}

type Hub struct {
	clients    map[*Client]bool
	channels   map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	mu         sync.RWMutex
	// metricsCache stores the latest dashboard:metrics payload per server_id so
	// new WebSocket clients receive current metrics immediately on subscribe
	// instead of waiting up to 30s for the next heartbeat.
	metricsCache   map[interface{}]Message
	metricsCacheMu sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*Client]bool),
		channels:     make(map[string]map[*Client]bool),
		register:     make(chan *Client),
		unregister:   make(chan *Client),
		broadcast:    make(chan Message, 256),
		metricsCache: make(map[interface{}]Message),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				for ch := range client.Channels {
					delete(h.channels[ch], client)
				}
				client.Conn.Close()
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			if clients, ok := h.channels[msg.Channel]; ok {
				data, _ := json.Marshal(msg)
				for client := range clients {
					client.mu.Lock()
					if err := client.Conn.WriteMessage(1, data); err != nil {
						slog.Error("ws write error", "error", err)
					}
					client.mu.Unlock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Subscribe(client *Client, channel string) {
	h.mu.Lock()
	if _, ok := h.channels[channel]; !ok {
		h.channels[channel] = make(map[*Client]bool)
	}
	h.channels[channel][client] = true
	client.Channels[channel] = true
	h.mu.Unlock()

	// Immediately push cached metrics to the new subscriber so the Dashboard
	// shows current data without waiting for the next 30s heartbeat.
	if channel == "dashboard:metrics" {
		h.metricsCacheMu.RLock()
		cached := make([]Message, 0, len(h.metricsCache))
		for _, msg := range h.metricsCache {
			cached = append(cached, msg)
		}
		h.metricsCacheMu.RUnlock()

		client.mu.Lock()
		for _, msg := range cached {
			if data, err := json.Marshal(msg); err == nil {
				_ = client.Conn.WriteMessage(1, data)
			}
		}
		client.mu.Unlock()
	}
}

func (h *Hub) Broadcast(channel string, msgType string, data interface{}) {
	msg := Message{Type: msgType, Channel: channel, Data: data}
	// Cache dashboard metrics per server_id for instant delivery to new subscribers
	if channel == "dashboard:metrics" {
		if m, ok := data.(map[string]interface{}); ok {
			if serverID := m["server_id"]; serverID != nil {
				h.metricsCacheMu.Lock()
				h.metricsCache[serverID] = msg
				h.metricsCacheMu.Unlock()
			}
		}
	}
	h.broadcast <- msg
}

func (h *Hub) Handler() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		client := &Client{
			Conn:     c,
			Channels: make(map[string]bool),
		}
		h.register <- client
		defer func() { h.unregister <- client }()

		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				break
			}
			var m struct {
				Action  string `json:"action"`
				Channel string `json:"channel"`
			}
			if json.Unmarshal(msg, &m) == nil {
				if m.Action == "subscribe" && m.Channel != "" {
					h.Subscribe(client, m.Channel)
				}
			}
		}
	})
}
