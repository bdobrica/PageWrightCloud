package handlers

import (
	"log"
	"net/http"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	gatewayWebsocket "github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/websocket"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, check against allowed origins
		return true
	},
}

type WebSocketHandler struct {
	hub *gatewayWebsocket.Hub
}

func NewWebSocketHandler(hub *gatewayWebsocket.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
	}
}

// HandleWebSocket upgrades HTTP connection to WebSocket
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user from context
	user, ok := middleware.GetUserFromContext(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create new client
	client := &gatewayWebsocket.Client{
		ID:     uuid.New().String(),
		UserID: user.UserID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Hub:    h.hub,
	}

	// Register client
	h.hub.Register <- client

	// Start read and write pumps
	go client.WritePump()
	go client.ReadPump()
}
