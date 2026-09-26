package websocket_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bengkol/backend/internal/websocket"
	"github.com/bengkol/backend/pkg/logger"
)

func setupTestHub() (*websocket.Hub, context.CancelFunc) {
	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)
	hub := websocket.NewHub(log)
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	return hub, cancel
}

func TestHub_LifecycleAndSubscription(t *testing.T) {
	hub, cancel := setupTestHub()
	defer cancel()

	var buf bytes.Buffer
	log := logger.NewWithOutput("development", "debug", &buf)

	client := websocket.NewClient(hub, nil, "user-123", "CUSTOMER", log)

	// Subscribe client to topic
	hub.Subscribe(client, "workshop:ws-1")

	// Allow hub loop to process
	time.Sleep(50 * time.Millisecond)

	if count := hub.SubscribedCount("workshop:ws-1"); count != 1 {
		t.Fatalf("expected subscribed count 1, got %d", count)
	}

	// Unsubscribe client
	hub.Unsubscribe(client, "workshop:ws-1")
	time.Sleep(50 * time.Millisecond)

	if count := hub.SubscribedCount("workshop:ws-1"); count != 0 {
		t.Fatalf("expected subscribed count 0, got %d", count)
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub, cancel := setupTestHub()
	defer cancel()

	// Direct broadcast verification without active clients (should not block or panic)
	hub.Broadcast("workshop:ws-1", websocket.EventQueueCalled, map[string]interface{}{
		"queue_number": 5,
	})

	time.Sleep(50 * time.Millisecond)
}

func TestEvents_TopicHelpers(t *testing.T) {
	if topic := websocket.TopicWorkshop("ws-100"); topic != "workshop:ws-100" {
		t.Errorf("expected workshop:ws-100, got %s", topic)
	}
	if topic := websocket.TopicUser("usr-200"); topic != "user:usr-200" {
		t.Errorf("expected user:usr-200, got %s", topic)
	}
	if topic := websocket.TopicBooking("bk-300"); topic != "booking:bk-300" {
		t.Errorf("expected booking:bk-300, got %s", topic)
	}
}

func TestOutboundMessageSerialization(t *testing.T) {
	now := time.Now()
	msg := websocket.OutboundMessage{
		Event:     websocket.EventQueueCalled,
		Topic:     "workshop:ws-1",
		Data:      map[string]int{"queue_number": 3},
		Timestamp: now,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal outbound message: %v", err)
	}

	var decoded websocket.OutboundMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal outbound message: %v", err)
	}

	if decoded.Event != websocket.EventQueueCalled {
		t.Errorf("expected event %s, got %s", websocket.EventQueueCalled, decoded.Event)
	}
	if decoded.Topic != "workshop:ws-1" {
		t.Errorf("expected topic workshop:ws-1, got %s", decoded.Topic)
	}
}
