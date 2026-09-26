package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/bengkol/backend/pkg/logger"
)

// Broadcaster provides an abstraction for emitting real-time events
type Broadcaster interface {
	Broadcast(topic string, event string, data interface{})
}

// Subscription represents a request to subscribe or unsubscribe from a topic
type Subscription struct {
	Client *Client
	Topic  string
}

// BroadcastMessage encapsulates payload and destination topic
type BroadcastMessage struct {
	Topic   string
	Message []byte
}

// Hub maintains the set of active clients and broadcasts messages to topic subscribers
type Hub struct {
	clients    map[*Client]bool
	topics     map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	subscribe  chan Subscription
	unsub      chan Subscription
	broadcast  chan BroadcastMessage
	mu         sync.RWMutex
	logger     *logger.Logger
}

// NewHub creates a new WebSocket Hub
func NewHub(log *logger.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		topics:     make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		subscribe:  make(chan Subscription),
		unsub:      make(chan Subscription),
		broadcast:  make(chan BroadcastMessage, 256),
		logger:     log,
	}
}

// Run starts the central event dispatch loop
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.topics = make(map[string]map[*Client]bool)
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("websocket client registered", "user_id", client.userID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)

				// Remove client from all subscribed topics
				for topic, clients := range h.topics {
					delete(clients, client)
					if len(clients) == 0 {
						delete(h.topics, topic)
					}
				}
			}
			h.mu.Unlock()
			h.logger.Debug("websocket client unregistered", "user_id", client.userID)

		case sub := <-h.subscribe:
			h.mu.Lock()
			if _, ok := h.topics[sub.Topic]; !ok {
				h.topics[sub.Topic] = make(map[*Client]bool)
			}
			h.topics[sub.Topic][sub.Client] = true
			h.mu.Unlock()
			h.logger.Debug("client subscribed to topic", "topic", sub.Topic, "user_id", sub.Client.userID)

		case unsub := <-h.unsub:
			h.mu.Lock()
			if clients, ok := h.topics[unsub.Topic]; ok {
				delete(clients, unsub.Client)
				if len(clients) == 0 {
					delete(h.topics, unsub.Topic)
				}
			}
			h.mu.Unlock()
			h.logger.Debug("client unsubscribed from topic", "topic", unsub.Topic, "user_id", unsub.Client.userID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients, ok := h.topics[msg.Topic]
			if ok {
				for client := range clients {
					select {
					case client.send <- msg.Message:
					default:
						// If client buffer is full, close and mark for cleanup
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast serializes and dispatches an event to all clients on a given topic
func (h *Hub) Broadcast(topic string, event string, data interface{}) {
	msg := OutboundMessage{
		Event:     event,
		Topic:     topic,
		Data:      data,
		Timestamp: time.Now(),
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		h.logger.Error("failed to marshal websocket broadcast message", "error", err, "event", event)
		return
	}

	h.broadcast <- BroadcastMessage{
		Topic:   topic,
		Message: bytes,
	}
}

// Subscribe adds a client to a topic
func (h *Hub) Subscribe(client *Client, topic string) {
	h.subscribe <- Subscription{
		Client: client,
		Topic:  topic,
	}
}

// Unsubscribe removes a client from a topic
func (h *Hub) Unsubscribe(client *Client, topic string) {
	h.unsub <- Subscription{
		Client: client,
		Topic:  topic,
	}
}

// ActiveConnections returns the count of connected clients
func (h *Hub) ActiveConnections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// SubscribedCount returns the count of clients on a specific topic
func (h *Hub) SubscribedCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.topics[topic])
}
