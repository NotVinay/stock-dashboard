package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/websocket"
)

func TestServer_SubscriptionManagement(t *testing.T) {
	tests := []struct {
		name                string
		operations          []subscribeOp
		expectedSubs        map[string]int
		expectedFinnhubMsgs []string
	}{
		{
			name: "first subscription to symbol",
			operations: []subscribeOp{
				{action: "subscribe", symbol: "AAPL"},
			},
			expectedSubs: map[string]int{"AAPL": 1},
			expectedFinnhubMsgs: []string{
				`{"symbol":"AAPL","type":"subscribe"}` + "\n",
			},
		},
		{
			name: "multiple subscriptions to same symbol",
			operations: []subscribeOp{
				{action: "subscribe", symbol: "AAPL"},
				{action: "subscribe", symbol: "AAPL"},
				{action: "subscribe", symbol: "AAPL"},
			},
			expectedSubs: map[string]int{"AAPL": 3},
			expectedFinnhubMsgs: []string{
				`{"symbol":"AAPL","type":"subscribe"}` + "\n", // Only first subscription sent to Finnhub
			},
		},
		{
			name: "unsubscribe when multiple subscribers",
			operations: []subscribeOp{
				{action: "subscribe", symbol: "AAPL"},
				{action: "subscribe", symbol: "AAPL"},
				{action: "unsubscribe", symbol: "AAPL"},
			},
			expectedSubs: map[string]int{"AAPL": 1},
			expectedFinnhubMsgs: []string{
				`{"symbol":"AAPL","type":"subscribe"}` + "\n",
				// No unsubscribe sent to Finnhub as there's still 1 subscriber
			},
		},
		{
			name: "unsubscribe last subscriber",
			operations: []subscribeOp{
				{action: "subscribe", symbol: "AAPL"},
				{action: "unsubscribe", symbol: "AAPL"},
			},
			expectedSubs: map[string]int{},
			expectedFinnhubMsgs: []string{
				`{"symbol":"AAPL","type":"subscribe"}` + "\n",
				`{"symbol":"AAPL","type":"unsubscribe"}` + "\n",
			},
		},
		{
			name: "multiple symbols",
			operations: []subscribeOp{
				{action: "subscribe", symbol: "AAPL"},
				{action: "subscribe", symbol: "GOOGL"},
				{action: "subscribe", symbol: "MSFT"},
				{action: "unsubscribe", symbol: "GOOGL"},
			},
			expectedSubs: map[string]int{"AAPL": 1, "MSFT": 1},
			expectedFinnhubMsgs: []string{
				`{"symbol":"AAPL","type":"subscribe"}` + "\n",
				`{"symbol":"GOOGL","type":"subscribe"}` + "\n",
				`{"symbol":"MSFT","type":"subscribe"}` + "\n",
				`{"symbol":"GOOGL","type":"unsubscribe"}` + "\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finnhubMessages := []string{}
			var mu sync.Mutex

			// Create mock Finnhub server
			mockFinnhub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()

				// Read messages from server
				go func() {
					for {
						_, msg, err := conn.ReadMessage()
						if err != nil {
							return
						}
						mu.Lock()
						finnhubMessages = append(finnhubMessages, string(msg))
						mu.Unlock()
					}
				}()

				// Keep connection alive
				<-time.After(500 * time.Millisecond)
			}))
			defer mockFinnhub.Close()

			// Create server
			finnhubURL := "ws" + strings.TrimPrefix(mockFinnhub.URL, "http")
			server := NewServer(finnhubURL)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := server.Start(ctx)
			if err != nil {
				t.Fatalf("Failed to start server: %v", err)
			}
			defer server.Shutdown()

			// Wait for connection to establish
			time.Sleep(50 * time.Millisecond)

			// Execute operations
			for _, op := range tt.operations {
				if op.action == "subscribe" {
					server.subscribeTo(op.symbol)
				} else {
					server.unsubscribeFrom(op.symbol)
				}
			}

			// Wait for messages to be sent
			time.Sleep(100 * time.Millisecond)

			// Verify subscriptions
			server.mu.Lock()
			actualSubs := make(map[string]int)
			for k, v := range server.subscriptions {
				actualSubs[k] = v
			}
			server.mu.Unlock()

			if diff := cmp.Diff(tt.expectedSubs, actualSubs); diff != "" {
				t.Errorf("Subscriptions mismatch (-want +got):\n%s", diff)
			}

			// Verify Finnhub messages
			mu.Lock()
			if diff := cmp.Diff(tt.expectedFinnhubMsgs, finnhubMessages); diff != "" {
				t.Errorf("Finnhub messages mismatch (-want +got):\n%s", diff)
			}
			mu.Unlock()
		})
	}
}

func TestServer_ProcessFinnhubMessage(t *testing.T) {
	tests := []struct {
		name               string
		finnhubMsg         map[string]interface{}
		subscribedSymbols  []string
		expectedBroadcasts map[string][]string // symbol -> messages
		expectPong         bool
	}{
		{
			name: "trade message for subscribed symbol",
			finnhubMsg: map[string]interface{}{
				"type": "trade",
				"data": []interface{}{
					map[string]interface{}{
						"s": "AAPL",
						"p": 150.25,
						"t": float64(1234567890),
						"v": float64(100),
					},
				},
			},
			subscribedSymbols: []string{"AAPL"},
			expectedBroadcasts: map[string][]string{
				"AAPL": {`{"type":"trade","data":{"s":"AAPL","p":150.25,"t":1234567890,"v":100}}`},
			},
		},
		{
			name: "trade message for unsubscribed symbol",
			finnhubMsg: map[string]interface{}{
				"type": "trade",
				"data": []interface{}{
					map[string]interface{}{
						"s": "GOOGL",
						"p": 2500.50,
						"t": float64(1234567890),
						"v": float64(50),
					},
				},
			},
			subscribedSymbols:  []string{"AAPL"},
			expectedBroadcasts: map[string][]string{}, // GOOGL broadcast still happens but no one subscribed
		},
		{
			name: "multiple trades in single message",
			finnhubMsg: map[string]interface{}{
				"type": "trade",
				"data": []interface{}{
					map[string]interface{}{
						"s": "AAPL",
						"p": 150.25,
						"t": float64(1234567890),
						"v": float64(100),
					},
					map[string]interface{}{
						"s": "GOOGL",
						"p": 2500.50,
						"t": float64(1234567891),
						"v": float64(50),
					},
				},
			},
			subscribedSymbols: []string{"AAPL", "GOOGL"},
			expectedBroadcasts: map[string][]string{
				"AAPL":  {`{"type":"trade","data":{"s":"AAPL","p":150.25,"t":1234567890,"v":100}}`},
				"GOOGL": {`{"type":"trade","data":{"s":"GOOGL","p":2500.5,"t":1234567891,"v":50}}`},
			},
		},
		{
			name: "ping message",
			finnhubMsg: map[string]interface{}{
				"type": "ping",
			},
			subscribedSymbols:  []string{},
			expectedBroadcasts: map[string][]string{},
			expectPong:         true,
		},
		{
			name: "invalid message type",
			finnhubMsg: map[string]interface{}{
				"type": "unknown",
				"data": "some data",
			},
			subscribedSymbols:  []string{},
			expectedBroadcasts: map[string][]string{},
		},
		{
			name: "trade with invalid data structure",
			finnhubMsg: map[string]interface{}{
				"type": "trade",
				"data": "invalid", // Should be array
			},
			subscribedSymbols:  []string{},
			expectedBroadcasts: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broadcasts := make(map[string][]string)
			var broadcastMu sync.Mutex
			pongReceived := false

			// Create mock Finnhub server to receive pong
			mockFinnhub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()

				// Read pong message
				go func() {
					for {
						_, msg, err := conn.ReadMessage()
						if err != nil {
							return
						}
						var m map[string]interface{}
						if err := json.Unmarshal(msg, &m); err == nil {
							if msgType, ok := m["type"].(string); ok && msgType == "pong" {
								pongReceived = true
							}
						}
					}
				}()

				<-time.After(200 * time.Millisecond)
			}))
			defer mockFinnhub.Close()

			finnhubURL := "ws" + strings.TrimPrefix(mockFinnhub.URL, "http")
			server := NewServer(finnhubURL)

			// Create test clients for each subscribed symbol
			for _, symbol := range tt.subscribedSymbols {
				client := &Client{
					send:          make(chan []byte, 256),
					subscriptions: map[string]bool{symbol: true},
				}
				server.hub.subscriptions[symbol] = map[*Client]bool{client: true}

				// Capture broadcasts for this symbol
				go func(sym string, c *Client) {
					for msg := range c.send {
						broadcastMu.Lock()
						broadcasts[sym] = append(broadcasts[sym], string(msg))
						broadcastMu.Unlock()
					}
				}(symbol, client)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := server.Start(ctx)
			if err != nil {
				t.Fatalf("Failed to start server: %v", err)
			}
			defer server.Shutdown()

			// Process the message
			server.processFinnhubMessage(tt.finnhubMsg)

			// Wait for async operations
			time.Sleep(50 * time.Millisecond)

			// Verify broadcasts
			broadcastMu.Lock()
			if diff := cmp.Diff(tt.expectedBroadcasts, broadcasts); diff != "" {
				t.Errorf("Broadcasts mismatch (-want +got):\n%s", diff)
			}
			broadcastMu.Unlock()

			// Verify pong
			if tt.expectPong && !pongReceived {
				t.Error("Expected pong message but didn't receive it")
			}
		})
	}
}

func TestServer_BroadcastToSubscribers(t *testing.T) {
	tests := []struct {
		name                string
		symbol              string
		message             string
		clientSubscriptions map[string][]string // clientID -> symbols
		expectedRecipients  []string            // client IDs that should receive the message
	}{
		{
			name:    "broadcast to single subscriber",
			symbol:  "AAPL",
			message: `{"type":"trade","symbol":"AAPL","price":150}`,
			clientSubscriptions: map[string][]string{
				"client1": {"AAPL"},
				"client2": {"GOOGL"},
			},
			expectedRecipients: []string{"client1"},
		},
		{
			name:    "broadcast to multiple subscribers",
			symbol:  "AAPL",
			message: `{"type":"trade","symbol":"AAPL","price":150}`,
			clientSubscriptions: map[string][]string{
				"client1": {"AAPL", "GOOGL"},
				"client2": {"AAPL"},
				"client3": {"MSFT"},
			},
			expectedRecipients: []string{"client1", "client2"},
		},
		{
			name:    "broadcast to no subscribers",
			symbol:  "TSLA",
			message: `{"type":"trade","symbol":"TSLA","price":800}`,
			clientSubscriptions: map[string][]string{
				"client1": {"AAPL"},
				"client2": {"GOOGL"},
			},
			expectedRecipients: []string{},
		},
		{
			name:                "broadcast with no clients",
			symbol:              "AAPL",
			message:             `{"type":"trade","symbol":"AAPL","price":150}`,
			clientSubscriptions: map[string][]string{},
			expectedRecipients:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer("ws://mock")
			clientMap := make(map[string]*Client)

			// Set up clients and subscriptions
			for clientID, symbols := range tt.clientSubscriptions {
				client := &Client{
					send:          make(chan []byte, 256),
					subscriptions: make(map[string]bool),
				}
				clientMap[clientID] = client

				for _, symbol := range symbols {
					client.subscriptions[symbol] = true
					server.hub.addSubscription(symbol, client)
				}
			}

			// Broadcast message
			server.broadcastToSubscribers(tt.symbol, []byte(tt.message))

			// Check which clients received the message
			actualRecipients := []string{}
			for clientID, client := range clientMap {
				select {
				case msg := <-client.send:
					if string(msg) == tt.message {
						actualRecipients = append(actualRecipients, clientID)
					}
				case <-time.After(50 * time.Millisecond):
					// Client didn't receive message
				}
			}

			// Sort for consistent comparison
			sortStrings(actualRecipients)
			sortStrings(tt.expectedRecipients)

			if diff := cmp.Diff(tt.expectedRecipients, actualRecipients); diff != "" {
				t.Errorf("Recipients mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestServer_HandleConnection(t *testing.T) {
	server := NewServer("ws://mock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server without Finnhub connection for this test
	server.wg = &sync.WaitGroup{}
	server.cancel = cancel
	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.hub.run(ctx)
	}()

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(server.HandleConnection))
	defer testServer.Close()

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Read welcome message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read welcome message: %v", err)
	}

	var welcome Message
	if err := json.Unmarshal(msg, &welcome); err != nil {
		t.Fatalf("Failed to unmarshal welcome message: %v", err)
	}

	if welcome.Type != "connected" {
		t.Errorf("Expected welcome message type 'connected', got '%s'", welcome.Type)
	}

	// Send subscribe message
	subMsg := SubscribeMessage{
		Type:    "subscribe",
		Symbols: []string{"AAPL"},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		t.Fatalf("Failed to send subscribe message: %v", err)
	}

	// Read confirmation
	_, confirmMsg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read confirmation: %v", err)
	}

	var confirmation Message
	if err := json.Unmarshal(confirmMsg, &confirmation); err != nil {
		t.Fatalf("Failed to unmarshal confirmation: %v", err)
	}

	if confirmation.Type != "subscribed" {
		t.Errorf("Expected confirmation type 'subscribed', got '%s'", confirmation.Type)
	}
}

func TestServer_GracefulShutdown(t *testing.T) {
	// Create mock Finnhub server
	mockFinnhub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-time.After(1 * time.Second)
	}))
	defer mockFinnhub.Close()

	finnhubURL := "ws" + strings.TrimPrefix(mockFinnhub.URL, "http")
	server := NewServer(finnhubURL)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Let server run for a bit
	time.Sleep(100 * time.Millisecond)

	// Shutdown server
	done := make(chan bool)
	go func() {
		server.Shutdown()
		done <- true
	}()

	// Verify shutdown completes in reasonable time
	select {
	case <-done:
		// Shutdown completed successfully
	case <-time.After(500 * time.Millisecond):
		t.Error("Server shutdown timed out")
	}

	// Verify finnhub connection is closed
	if server.finnhubConn != nil {
		// Try to write to connection to check if it's closed
		err := server.finnhubConn.WriteMessage(websocket.PingMessage, nil)
		if err == nil {
			t.Error("Finnhub connection not closed after shutdown")
		}
	}
}

// Helper types

type subscribeOp struct {
	action string
	symbol string
}
