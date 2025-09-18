package websocket

import (
	"context"
	"log"
	"sync"
)

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	// New field for efficient broadcasting: maps a symbol to a set of clients.
	subscriptions map[string]map[*Client]bool
	mu            sync.RWMutex
}

// newHub creates a new Hub instance.
func newHub() *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		broadcast:     make(chan []byte, 256),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		subscriptions: make(map[string]map[*Client]bool),
	}
}

// run starts the hub's event loop for managing clients and messages.
func (h *Hub) run(ctx context.Context) {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client connected. Total clients: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)

				// Also remove the client from all subscription maps to prevent memory leaks.
				client.mu.RLock()
				for symbol := range client.subscriptions {
					if _, ok := h.subscriptions[symbol]; ok {
						delete(h.subscriptions[symbol], client)
						// If the symbol has no more subscribers, remove the symbol entry itself.
						if len(h.subscriptions[symbol]) == 0 {
							delete(h.subscriptions, symbol)
						}
					}
				}
				client.mu.RUnlock()

				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("Client disconnected. Total clients: %d", len(h.clients))

		case message := <-h.broadcast:
			// This broadcast logic is not currently used by broadcastToSubscribers,
			// but is kept for potential future use (e.g., broadcasting to all clients).
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// If the send buffer is full, assume the client is dead or stuck.
					delete(h.clients, client)
					close(client.send)
				}
			}
			h.mu.RUnlock()

		case <-ctx.Done():
			// Close all client connections on shutdown
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return // Exit the loop
		}
	}
}

// addSubscription adds a client to a symbol's subscription list.
func (h *Hub) addSubscription(symbol string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscriptions[symbol]; !ok {
		h.subscriptions[symbol] = make(map[*Client]bool)
	}
	h.subscriptions[symbol][client] = true
}

// removeSubscription removes a client from a symbol's subscription list.
func (h *Hub) removeSubscription(symbol string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.subscriptions[symbol]; ok {
		delete(subs, client)
		if len(subs) == 0 {
			delete(h.subscriptions, symbol)
		}
	}
}
