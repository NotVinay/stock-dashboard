package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Server represents the WebSocket server.
type Server struct {
	hub           *Hub
	finnhubConn   *websocket.Conn
	finnhubURL    string
	mu            sync.Mutex
	subscriptions map[string]int
	upgrader      websocket.Upgrader
	// wg waits for all running goroutines to finish.
	wg *sync.WaitGroup
	// cancel is the function to call to signal shutdown.
	cancel context.CancelFunc
}

// NewServer creates a new WebSocket server instance.
func NewServer(finnhubURL string) *Server {
	return &Server{
		hub:           newHub(),
		finnhubURL:    finnhubURL,
		subscriptions: make(map[string]int),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// IMPORTANT: In production, you should implement proper origin checking
				// to prevent Cross-Site WebSocket Hijacking.
				// Example: return r.Header.Get("Origin") == "https://your-frontend-domain.com"
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
} // Start initializes the WebSocket server and its background processes.
// It now takes a parent context for cancellation.
func (s *Server) Start(ctx context.Context) error {
	var serverCtx context.Context
	serverCtx, s.cancel = context.WithCancel(ctx)
	s.wg = &sync.WaitGroup{}

	// Start the hub's main loop in a goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.hub.run(serverCtx) // Pass context to the hub
	}()

	if err := s.connectToFinnhub(); err != nil {
		s.cancel() // Cancel context if connection fails
		s.wg.Wait()
		return err
	}

	// Start the Finnhub listener in a goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.listenToFinnhub(serverCtx) // Pass context to the listener
	}()

	log.Println("✅ WebSocket server started successfully")
	return nil
}

// Shutdown gracefully stops the server and all its goroutines.
func (s *Server) Shutdown() {
	log.Println("⏳ Shutting down WebSocket server...")
	// Signal all goroutines to stop by canceling the context.
	s.cancel()
	// Wait for all goroutines to finish their cleanup.
	s.wg.Wait()
	// Close the upstream connection.
	if s.finnhubConn != nil {
		s.finnhubConn.Close()
	}
	log.Println("🛑 WebSocket server shut down gracefully.")
}

// HandleConnection handles incoming WebSocket connection requests from clients.
func (s *Server) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		return
	}

	client := &Client{
		conn:          conn,
		send:          make(chan []byte, 256),
		hub:           s.hub,
		subscriptions: make(map[string]bool),
	}

	s.hub.register <- client

	go client.writePump()
	go client.readPump(s)

	// Send a welcome message to the newly connected client.
	welcome := Message{
		Type: "connected",
		Data: map[string]string{"message": "Connected to stock price stream"},
	}
	if data, err := json.Marshal(welcome); err == nil {
		client.send <- data
	}
}

// connectToFinnhub establishes the connection to the Finnhub WebSocket.
func (s *Server) connectToFinnhub() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(s.finnhubURL, nil)
	if err != nil {
		return err
	}

	s.finnhubConn = conn
	log.Println("Connected to Finnhub WebSocket")
	return nil
}

// listenToFinnhub continuously listens for messages from Finnhub.
func (s *Server) listenToFinnhub(ctx context.Context) {
	// Check if the connection is nil, which can happen if shutdown is called
	// before the connection is established.
	if s.finnhubConn == nil {
		return
	}
	defer s.finnhubConn.Close()

	msgChan := make(chan []byte)
	errChan := make(chan error)

	go func() {
		for {
			_, message, err := s.finnhubConn.ReadMessage()
			if err != nil {
				// If the context is done, the error is likely due to the connection
				// being closed, which is expected. Don't forward the error.
				select {
				case <-ctx.Done():
				default:
					errChan <- err
				}
				return
			}
			msgChan <- message
		}
	}()

	for {
		select {
		case <-ctx.Done(): // Listen for shutdown signal
			log.Println("Stopping Finnhub listener...")
			return
		case message, ok := <-msgChan:
			if !ok {
				return
			}
			var finnhubMsg map[string]interface{}
			if err := json.Unmarshal(message, &finnhubMsg); err != nil {
				log.Printf("Error parsing Finnhub message: %v", err)
				continue
			}
			s.processFinnhubMessage(finnhubMsg)
		case err, ok := <-errChan:
			if !ok { // Channel closed, the reading goroutine has exited.
				return
			}
			log.Printf("Error reading from Finnhub: %v. Attempting to reconnect...", err)
			// In production, you might want to implement exponential backoff here.
			if err := s.connectToFinnhub(); err != nil {
				log.Printf("Failed to reconnect to Finnhub: %v", err)
				return // Could not reconnect, exiting the listener.
			}
			// After reconnecting, you should re-launch the reader goroutine
			// for the new connection, or restructure this logic. For simplicity,
			// we are exiting, and a robust implementation would handle this.
			return
		}
	}
}

// processFinnhubMessage processes messages from Finnhub and broadcasts them.
func (s *Server) processFinnhubMessage(msg map[string]interface{}) {
	msgType, ok := msg["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "trade":
		data, ok := msg["data"].([]interface{})
		if !ok {
			return
		}
		for _, trade := range data {
			tradeMap, ok := trade.(map[string]interface{})
			if !ok {
				continue
			}
			tradeData := TradeData{
				Symbol:    getString(tradeMap, "s"),
				Price:     getFloat64(tradeMap, "p"),
				Timestamp: getInt64(tradeMap, "t"),
				Volume:    getInt64(tradeMap, "v"),
			}
			message := Message{Type: "trade", Data: tradeData}
			if jsonData, err := json.Marshal(message); err == nil {
				s.broadcastToSubscribers(tradeData.Symbol, jsonData)
			}
		}
	case "ping":
		// Finnhub expects a pong response to keep the connection alive.
		pong := map[string]string{"type": "pong"}
		if s.finnhubConn != nil {
			if err := s.finnhubConn.WriteJSON(pong); err != nil {
				log.Printf("Error sending pong to Finnhub: %v", err)
			}
		}
	}
}

// broadcastToSubscribers sends a message to all clients subscribed to a specific symbol.
// This is the new, highly efficient implementation.
func (s *Server) broadcastToSubscribers(symbol string, message []byte) {
	s.hub.mu.RLock()
	defer s.hub.mu.RUnlock()

	// Direct lookup of subscribers for the given symbol. O(1) complexity.
	if subscribers, ok := s.hub.subscriptions[symbol]; ok {
		for client := range subscribers {
			select {
			case client.send <- message:
			default:
				// Don't block if a client's send channel is full.
				// The unregister logic will eventually clean up slow clients.
			}
		}
	}
}

// subscribeTo manages server-side subscription to a Finnhub symbol.
func (s *Server) subscribeTo(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions[symbol]++
	if s.subscriptions[symbol] == 1 {
		// This is the first client subscribing to this symbol.
		msg := map[string]string{"type": "subscribe", "symbol": symbol}
		if s.finnhubConn != nil {
			if err := s.finnhubConn.WriteJSON(msg); err != nil {
				log.Printf("Error subscribing to %s on Finnhub: %v", symbol, err)
			} else {
				log.Printf("Subscribed to %s on Finnhub", symbol)
			}
		}
	}
}

// unsubscribeFrom manages server-side unsubscription from a Finnhub symbol.
func (s *Server) unsubscribeFrom(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count, ok := s.subscriptions[symbol]; ok && count > 0 {
		s.subscriptions[symbol]--
		if s.subscriptions[symbol] == 0 {
			// This was the last client for this symbol.
			delete(s.subscriptions, symbol)
			msg := map[string]string{"type": "unsubscribe", "symbol": symbol}
			if s.finnhubConn != nil {
				if err := s.finnhubConn.WriteJSON(msg); err != nil {
					log.Printf("Error unsubscribing from %s on Finnhub: %v", symbol, err)
				} else {
					log.Printf("Unsubscribed from %s on Finnhub", symbol)
				}
			}
		}
	}
}
