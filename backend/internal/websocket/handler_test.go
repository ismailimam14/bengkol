package websocket_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/domain"
	ws "github.com/bengkol/backend/internal/websocket"
	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/security"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func TestWebSocketHandler_ServeWS_Unauthorized(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	hub := ws.NewHub(log)
	handler := ws.NewHandler(hub, jwtMgr, log)

	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Attempt to connect without token
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected dial error due to missing token, but succeeded")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code 401 Unauthorized, got %d", resp.StatusCode)
	}
}

func TestWebSocketHandler_ServeWS_SuccessAndPingPong(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	jwtMgr := security.NewJWTManager("test-secret-at-least-32-chars-long", "test-rf", 15, 7)
	hub := ws.NewHub(log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	handler := ws.NewHandler(hub, jwtMgr, log)
	server := httptest.NewServer(http.HandlerFunc(handler.ServeWS))
	defer server.Close()

	userID := uuid.New()
	user := &domain.User{
		ID:    userID,
		Email: "customer@example.com",
		Name:  "Test Customer",
		Role:  domain.RoleCustomer,
	}

	tokens, _, err := jwtMgr.GenerateTokenPair(user)
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + tokens.AccessToken

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect to websocket: %v (status: %v)", err, resp.StatusCode)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols, got %d", resp.StatusCode)
	}

	// Send subscribe action
	subMsg := ws.InboundMessage{
		Action: "subscribe",
		Topic:  "workshop:test-ws-1",
	}
	subBytes, _ := json.Marshal(subMsg)
	if err := conn.WriteMessage(websocket.TextMessage, subBytes); err != nil {
		t.Fatalf("failed to send subscribe message: %v", err)
	}

	// Read ack message
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgBytes, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var outMsg ws.OutboundMessage
	if err := json.Unmarshal(msgBytes, &outMsg); err != nil {
		t.Fatalf("failed to parse outbound message: %v", err)
	}

	if outMsg.Event != ws.EventSubscribed || outMsg.Topic != "workshop:test-ws-1" {
		t.Errorf("expected SUBSCRIBED to workshop:test-ws-1, got %s for topic %s", outMsg.Event, outMsg.Topic)
	}

	// Test broadcast from server
	hub.Broadcast("workshop:test-ws-1", ws.EventQueueCalled, map[string]interface{}{
		"queue_number": 7,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, broadcastBytes, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read broadcast message: %v", err)
	}

	var broadcastMsg ws.OutboundMessage
	if err := json.Unmarshal(broadcastBytes, &broadcastMsg); err != nil {
		t.Fatalf("failed to parse broadcast message: %v", err)
	}

	if broadcastMsg.Event != ws.EventQueueCalled {
		t.Errorf("expected event QUEUE_CALLED, got %s", broadcastMsg.Event)
	}
}
