package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

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
	maxMessageSize = 512
)

// Client is a middleman between the WebSocket connection and the hub.
type Client struct {
	conn *websocket.Conn
	send chan []byte
	hub  *Hub

	// subscriptions maps a symbol (e.g., "AAPL") to true if subscribed.
	subscriptions map[string]bool
	mu            sync.RWMutex
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *Client) readPump(server *Server) {
	defer func() {
		// On exit, ensure all client's subscriptions are removed from the server and hub.
		c.mu.RLock()
		for symbol := range c.subscriptions {
			server.unsubscribeFrom(symbol)
			server.hub.removeSubscription(symbol, c) // Also update the hub's map
		}
		c.mu.RUnlock()

		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg SubscribeMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error parsing client message: %v", err)
			continue
		}

		// Handle subscription/unsubscription requests
		switch msg.Type {
		case "subscribe":
			c.handleSubscription(msg, server, true)
		case "unsubscribe":
			c.handleSubscription(msg, server, false)
		}
	}
}

// handleSubscription manages the subscription state for the client and server.
func (c *Client) handleSubscription(msg SubscribeMessage, server *Server, subscribe bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, symbol := range msg.Symbols {
		if subscribe {
			if !c.subscriptions[symbol] {
				c.subscriptions[symbol] = true
				server.subscribeTo(symbol)
				server.hub.addSubscription(symbol, c) // Tell the hub to add this client
			}
		} else {
			if c.subscriptions[symbol] {
				delete(c.subscriptions, symbol)
				server.unsubscribeFrom(symbol)
				server.hub.removeSubscription(symbol, c) // Tell the hub to remove this client
			}
		}
	}

	// Send a confirmation message back to the client.
	responseType := "subscribed"
	if !subscribe {
		responseType = "unsubscribed"
	}
	response := Message{
		Type: responseType,
		Data: msg.Symbols,
	}
	if data, err := json.Marshal(response); err == nil {
		c.send <- data
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			// Send a ping message to the client to keep the connection alive.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Connection is likely dead.
			}
		}
	}
}
