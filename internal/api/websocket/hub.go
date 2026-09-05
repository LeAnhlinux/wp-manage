package websocket

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

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

// writeTimeout bounds a single WebSocket write. Without a deadline, a client on a
// stalled connection (dead mobile network, suspended laptop) fills its TCP send
// buffer and WriteMessage blocks indefinitely. That stops the hub's broadcast
// loop while it still holds h.mu, so Subscribe and register/unregister block too;
// the broadcast channel then fills and Broadcast() starts blocking its caller —
// the agent heartbeat handler. One stuck browser tab could stall heartbeats for
// every agent on the panel.
const writeTimeout = 5 * time.Second

// write sends one message with a deadline. A returned error means the client is
// unresponsive and should be dropped.
func (c *Client) write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.Conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return c.Conn.WriteMessage(websocket.TextMessage, data)
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
			h.removeClient(client)

		case msg := <-h.broadcast:
			// Collect unresponsive clients while holding only the read lock, then
			// drop them afterwards — removeClient takes the write lock, and this is
			// the same goroutine that services h.unregister, so it must not send
			// there (that would deadlock).
			var dead []*Client
			h.mu.RLock()
			if clients, ok := h.channels[msg.Channel]; ok {
				data, _ := json.Marshal(msg)
				for client := range clients {
					if err := client.write(data); err != nil {
						slog.Warn("ws write failed, dropping client", "channel", msg.Channel, "error", err)
						dead = append(dead, client)
					}
				}
			}
			h.mu.RUnlock()
			for _, client := range dead {
				h.removeClient(client)
			}
		}
	}
}

// removeClient unsubscribes a client from every channel and closes its socket.
// Safe to call for a client that was already removed.
func (h *Hub) removeClient(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		for ch := range client.Channels {
			delete(h.channels[ch], client)
		}
		client.Conn.Close()
	}
	h.mu.Unlock()
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

		for _, msg := range cached {
			if data, err := json.Marshal(msg); err == nil {
				if err := client.write(data); err != nil {
					slog.Warn("ws cache replay failed, dropping client", "error", err)
					h.removeClient(client)
					return
				}
			}
		}
	}
}

// EvictServerMetrics removes a server's cached dashboard metric entry.
// Call this when a server is deleted so stale data is not pushed to new WS clients.
func (h *Hub) EvictServerMetrics(serverID uint64) {
	h.metricsCacheMu.Lock()
	// metricsCache key is interface{} stored as the JSON-decoded server_id value,
	// which comes through as float64 from JSON. Try both uint64 and float64 forms.
	delete(h.metricsCache, serverID)
	delete(h.metricsCache, float64(serverID))
	h.metricsCacheMu.Unlock()
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
	// Never block the caller. Broadcast runs inside request handlers (notably the
	// agent heartbeat handler, twice per heartbeat); if the hub is wedged or
	// saturated, dropping a metrics frame is far better than stalling every
	// agent's heartbeat behind it. Metrics are re-sent on the next heartbeat and
	// the latest value is already in metricsCache above.
	select {
	case h.broadcast <- msg:
	default:
		slog.Warn("ws broadcast queue full, dropping message", "channel", channel, "type", msgType)
	}
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
