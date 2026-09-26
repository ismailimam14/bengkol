package websocket

import (
	"net/http"
	"strings"

	"github.com/bengkol/backend/pkg/logger"
	"github.com/bengkol/backend/pkg/response"
	"github.com/bengkol/backend/pkg/security"
	"github.com/gorilla/websocket"
)

var defaultUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for mobile client and cross-origin frontend
		return true
	},
}

// Handler manages WebSocket upgrades and connection lifecycles.
type Handler struct {
	hub      *Hub
	jwtMgr   *security.JWTManager
	logger   *logger.Logger
	upgrader websocket.Upgrader
}

// NewHandler creates a new WebSocket handler instance.
func NewHandler(hub *Hub, jwtMgr *security.JWTManager, log *logger.Logger) *Handler {
	return &Handler{
		hub:      hub,
		jwtMgr:   jwtMgr,
		logger:   log,
		upgrader: defaultUpgrader,
	}
}

// ServeWS handles incoming WebSocket upgrade requests.
// Authentication token can be passed via `?token=` query param or `Authorization: Bearer <token>` header.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				tokenString = parts[1]
			}
		}
	}

	if tokenString == "" {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Authentication token required for WebSocket connection")
		return
	}

	claims, err := h.jwtMgr.ValidateAccessToken(tokenString)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, response.ErrCodeUnauthorized, "Invalid or expired authentication token")
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket connection upgrade failed", "error", err, "user_id", claims.UserID)
		return
	}

	client := NewClient(h.hub, conn, claims.UserID.String(), string(claims.Role), h.logger)
	h.hub.register <- client

	// Auto-subscribe to the user's personal notifications topic
	h.hub.Subscribe(client, TopicUser(claims.UserID.String()))

	go client.WritePump()
	go client.ReadPump()

	h.logger.Info("websocket client connected and initialized", "user_id", claims.UserID, "role", claims.Role)
}
