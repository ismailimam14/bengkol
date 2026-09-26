package websocket

import "time"

// Event types for real-time messaging
const (
	EventPing                 = "PING"
	EventPong                 = "PONG"
	EventSubscribed           = "SUBSCRIBED"
	EventUnsubscribed         = "UNSUBSCRIBED"
	EventError                = "ERROR"
	EventQueueCheckIn         = "QUEUE_CHECKED_IN"
	EventQueueCalled          = "QUEUE_CALLED"
	EventQueueStarted         = "QUEUE_STARTED"
	EventQueueCompleted       = "QUEUE_COMPLETED"
	EventQueueNoShow          = "QUEUE_NO_SHOW"
	EventQueueCancelled       = "QUEUE_CANCELLED"
	EventQueueSummaryUpdated  = "QUEUE_SUMMARY_UPDATED"
	EventBookingCreated       = "BOOKING_CREATED"
	EventBookingCancelled     = "BOOKING_CANCELLED"
)

// InboundMessage represents a message sent from client to server
type InboundMessage struct {
	Action string `json:"action"` // "subscribe", "unsubscribe", "ping"
	Topic  string `json:"topic"`  // e.g. "workshop:UUID", "user:UUID"
}

// OutboundMessage represents a message sent from server to client
type OutboundMessage struct {
	Event     string      `json:"event"`
	Topic     string      `json:"topic,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// Helper topic builders
func TopicWorkshop(workshopID string) string {
	return "workshop:" + workshopID
}

func TopicUser(userID string) string {
	return "user:" + userID
}

func TopicBooking(bookingID string) string {
	return "booking:" + bookingID
}
