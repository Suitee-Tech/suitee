package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// Client represents an active WebSocket connection
type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	TenantID string
	UserID   string
	Role     string
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients mapped by TenantID and UserID
	// map[tenantID]map[userID]map[*Client]bool
	clients map[string]map[string]map[*Client]bool

	// Register requests from the clients
	Register chan *Client

	// Unregister requests from clients
	Unregister chan *Client

	// Webhook endpoint to call on new message (simplified for Phase 4)
	WebhookURL string

	mu sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[string]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		WebhookURL: "https://mock-push-service.toca.com/webhook", // Mock URL for mobile push
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			if h.clients[client.TenantID] == nil {
				h.clients[client.TenantID] = make(map[string]map[*Client]bool)
			}
			if h.clients[client.TenantID][client.UserID] == nil {
				h.clients[client.TenantID][client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.TenantID][client.UserID][client] = true
			h.mu.Unlock()

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.TenantID][client.UserID][client]; ok {
				delete(h.clients[client.TenantID][client.UserID], client)
				close(client.Send)
				if len(h.clients[client.TenantID][client.UserID]) == 0 {
					delete(h.clients[client.TenantID], client.UserID)
				}
				if len(h.clients[client.TenantID]) == 0 {
					delete(h.clients, client.TenantID)
				}
			}
			h.mu.Unlock()
		}
	}
}

// WSEvent represents an event to be sent over WebSocket
type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// triggerWebhook asynchronously calls a webhook to notify offline clients
func (h *Hub) triggerWebhook(tenantID string, userID string, msg *message.Message) {
	if h.WebhookURL == "" {
		return
	}

	go func() {
		payload := map[string]interface{}{
			"tenant_id": tenantID,
			"user_id":   userID,
			"message":   msg,
		}
		data, _ := json.Marshal(payload)

		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequest("POST", h.WebhookURL, bytes.NewBuffer(data))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Ignore response in fire-and-forget
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

// BroadcastMessage sends a message to specific users within a tenant
func (h *Hub) BroadcastMessage(tenantID string, userIDs []string, msg *message.Message) {
	event := WSEvent{
		Type:    "new_message",
		Payload: msg,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	send := func(client *Client) bool {
		select {
		case client.Send <- data:
			return true
		default:
			close(client.Send)
			return false
		}
	}

	seen := make(map[*Client]bool)
	deliver := func(userClients map[*Client]bool) {
		for client := range userClients {
			if seen[client] {
				continue
			}
			seen[client] = true
			if !send(client) {
				delete(userClients, client)
			}
		}
	}

	// 1) Entrega pelo tenant do canal (caminho padrão).
	if tenantClients, ok := h.clients[tenantID]; ok {
		for _, userID := range userIDs {
			if userClients, ok := tenantClients[userID]; ok {
				deliver(userClients)
			}
		}
	}

	// 2) Fallback: entrega por userID em QUALQUER tenant (cliente OTP com
	// tenant do token divergente do canal — sem isso o tempo real não chega).
	for _, userID := range userIDs {
		for _, tenantClients := range h.clients {
			if userClients, ok := tenantClients[userID]; ok {
				deliver(userClients)
			}
		}
	}

	// Push notification para participantes sem conexão (mantém o webhook).
	for _, userID := range userIDs {
		h.triggerWebhook(tenantID, userID, msg)
	}
}

// BroadcastEvent sends a generic event to specific users within a tenant
func (h *Hub) BroadcastEvent(tenantID string, userIDs []string, eventType string, payload interface{}) {
	event := WSEvent{
		Type:    eventType,
		Payload: payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	send := func(client *Client) bool {
		select {
		case client.Send <- data:
			return true
		default:
			close(client.Send)
			return false
		}
	}

	seen := make(map[*Client]bool)
	deliver := func(userClients map[*Client]bool) {
		for client := range userClients {
			if seen[client] {
				continue
			}
			seen[client] = true
			if !send(client) {
				delete(userClients, client)
			}
		}
	}

	// 1) Entrega pelo tenant do canal (caminho padrão).
	if tenantClients, ok := h.clients[tenantID]; ok {
		for _, userID := range userIDs {
			if userClients, ok := tenantClients[userID]; ok {
				deliver(userClients)
			}
		}
	}

	// 2) Fallback: entrega por userID em QUALQUER tenant (cliente do portal
	// pode ter tenant do token divergente do canal — sem isso ele não recebe
	// mensagens em tempo real).
	for _, userID := range userIDs {
		for _, tenantClients := range h.clients {
			if userClients, ok := tenantClients[userID]; ok {
				deliver(userClients)
			}
		}
	}
}

// BroadcastEventToTenant sends a generic event to every connected client of a tenant,
// regardless of user ID. Use this for monitoring/live-feed events that multiple roles
// (admins, managers, the assigned user) need to observe simultaneously.
func (h *Hub) BroadcastEventToTenant(tenantID string, eventType string, payload interface{}) {
	event := WSEvent{
		Type:    eventType,
		Payload: payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	tenantClients, ok := h.clients[tenantID]
	if !ok {
		return
	}

	for _, userClients := range tenantClients {
		for client := range userClients {
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(userClients, client)
			}
		}
	}
}

func (h *Hub) BroadcastEventToTenantInternal(tenantID string, eventType string, payload interface{}) {
	event := WSEvent{
		Type:    eventType,
		Payload: payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	tenantClients, ok := h.clients[tenantID]
	if !ok {
		return
	}

	for _, userClients := range tenantClients {
		for client := range userClients {
			if client.Role == "cliente" {
				continue
			}
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(userClients, client)
			}
		}
	}
}

// GetTenantUserIDs returns all connected user IDs for a given tenant
func (h *Hub) GetTenantUserIDs(tenantID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tenantClients, ok := h.clients[tenantID]
	if !ok {
		return nil
	}

	userIDs := make([]string, 0, len(tenantClients))
	for userID := range tenantClients {
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

// IsUserOnline reports whether the user has at least one active WebSocket
// connection for the tenant. Used by the transactional email notifier to
// suppress emails for users who are seeing the in-app notification in real
// time (docs/features/comunicacao/notificacoes-email-beta.md, regra 2).
func (h *Hub) IsUserOnline(tenantID string, userID string) bool {
	if h == nil || tenantID == "" || userID == "" {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	tenantClients, ok := h.clients[tenantID]
	if !ok {
		return false
	}
	userClients, ok := tenantClients[userID]
	return ok && len(userClients) > 0
}

// Write pump for a client
func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()
	for message := range c.Send {
		w, err := c.Conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return
		}
		_, _ = w.Write(message)

		// Add queued chat messages to the current websocket message
		n := len(c.Send)
		for i := 0; i < n; i++ {
			_, _ = w.Write([]byte{'\n'})
			_, _ = w.Write(<-c.Send)
		}

		if err := w.Close(); err != nil {
			return
		}
	}

	// If the channel is closed, send a close message to the websocket
	_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
}

// Read pump for a client (mainly to handle ping/pong and disconnects)
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
