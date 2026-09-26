package websocket

import (
	"encoding/json"
	"time"

	"github.com/bengkol/backend/pkg/logger"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 1024
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
	role   string
	logger *logger.Logger
}

// NewClient creates a new Client instance.
func NewClient(hub *Hub, conn *websocket.Conn, userID, role string, log *logger.Logger) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
		role:   role,
		logger: log,
	}
}

// ReadPump pumps messages from the websocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				c.logger.Warn("websocket unexpected close error", "error", err, "user_id", c.userID)
			}
			break
		}

		var inMsg InboundMessage
		if err := json.Unmarshal(message, &inMsg); err != nil {
			c.sendError("invalid JSON payload")
			continue
		}

		switch inMsg.Action {
		case "subscribe":
			if inMsg.Topic == "" {
				c.sendError("topic is required for subscription")
				continue
			}
			c.hub.Subscribe(c, inMsg.Topic)
			c.sendAck(EventSubscribed, inMsg.Topic)

		case "unsubscribe":
			if inMsg.Topic == "" {
				c.sendError("topic is required for unsubscription")
				continue
			}
			c.hub.Unsubscribe(c, inMsg.Topic)
			c.sendAck(EventUnsubscribed, inMsg.Topic)

		case "ping":
			c.sendAck(EventPong, "")

		default:
			c.sendError("unknown action: " + inMsg.Action)
		}
	}
}

// WritePump pumps messages from the hub to the websocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			// Add queued chat messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write([]byte{'\n'}); err != nil {
					return
				}
				if _, err := w.Write(<-c.send); err != nil {
					return
				}
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) sendAck(event string, topic string) {
	out := OutboundMessage{
		Event:     event,
		Topic:     topic,
		Timestamp: time.Now(),
	}
	bytes, err := json.Marshal(out)
	if err == nil {
		select {
		case c.send <- bytes:
		default:
		}
	}
}

func (c *Client) sendError(errMsg string) {
	out := OutboundMessage{
		Event:     EventError,
		Data:      map[string]string{"error": errMsg},
		Timestamp: time.Now(),
	}
	bytes, err := json.Marshal(out)
	if err == nil {
		select {
		case c.send <- bytes:
		default:
		}
	}
}
