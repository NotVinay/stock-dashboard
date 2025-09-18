package websocket

import (
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

func TestClient_HandleSubscription(t *testing.T) {
	tests := []struct {
		name               string
		msg                SubscribeMessage
		initialSubs        map[string]bool
		subscribe          bool
		expectedSubs       map[string]bool
		expectedServerSubs map[string]int // Expected server subscription counts
		expectedHubSubs    map[string]int // Expected hub subscription counts per symbol
	}{
		{
			name: "subscribe to new symbols",
			msg: SubscribeMessage{
				Type:    "subscribe",
				Symbols: []string{"AAPL", "GOOGL"},
			},
			initialSubs:        map[string]bool{},
			subscribe:          true,
			expectedSubs:       map[string]bool{"AAPL": true, "GOOGL": true},
			expectedServerSubs: map[string]int{"AAPL": 1, "GOOGL": 1},
			expectedHubSubs:    map[string]int{"AAPL": 1, "GOOGL": 1},
		},
		{
			name: "subscribe to already subscribed symbol",
			msg: SubscribeMessage{
				Type:    "subscribe",
				Symbols: []string{"AAPL"},
			},
			initialSubs:        map[string]bool{"AAPL": true},
			subscribe:          true,
			expectedSubs:       map[string]bool{"AAPL": true},
			expectedServerSubs: map[string]int{}, // No change in server subscriptions
			expectedHubSubs:    map[string]int{}, // No change in hub subscriptions
		},
		{
			name: "unsubscribe from existing symbols",
			msg: SubscribeMessage{
				Type:    "unsubscribe",
				Symbols: []string{"AAPL", "GOOGL"},
			},
			initialSubs:        map[string]bool{"AAPL": true, "GOOGL": true, "MSFT": true},
			subscribe:          false,
			expectedSubs:       map[string]bool{"MSFT": true},
			expectedServerSubs: map[string]int{},          // Will be checked differently for unsubscribe
			expectedHubSubs:    map[string]int{"MSFT": 0}, // MSFT remains, others removed
		},
		{
			name: "unsubscribe from non-subscribed symbol",
			msg: SubscribeMessage{
				Type:    "unsubscribe",
				Symbols: []string{"AAPL"},
			},
			initialSubs:        map[string]bool{"GOOGL": true},
			subscribe:          false,
			expectedSubs:       map[string]bool{"GOOGL": true},
			expectedServerSubs: map[string]int{},
			expectedHubSubs:    map[string]int{"GOOGL": 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create real server with hub
			server := &Server{
				subscriptions: make(map[string]int),
				hub: &Hub{
					subscriptions: make(map[string]map[*Client]bool),
				},
			}

			client := &Client{
				send:          make(chan []byte, 10),
				subscriptions: copySubscriptions(tt.initialSubs),
				mu:            sync.RWMutex{},
			}

			// Execute
			client.handleSubscription(tt.msg, server, tt.subscribe)

			// Verify client subscriptions
			if diff := cmp.Diff(tt.expectedSubs, client.subscriptions); diff != "" {
				t.Errorf("Client subscriptions mismatch (-want +got):\n%s", diff)
			}

			// Verify server subscriptions (for subscribe operations)
			if tt.subscribe {
				for symbol, expectedCount := range tt.expectedServerSubs {
					if server.subscriptions[symbol] != expectedCount {
						t.Errorf("Server subscription count for %s: got %d, want %d",
							symbol, server.subscriptions[symbol], expectedCount)
					}
				}
			}

			// Verify hub subscriptions
			for symbol, expectedCount := range tt.expectedHubSubs {
				actualCount := 0
				if clients, exists := server.hub.subscriptions[symbol]; exists {
					actualCount = len(clients)
				}
				if actualCount != expectedCount {
					t.Errorf("Hub subscription count for %s: got %d, want %d",
						symbol, actualCount, expectedCount)
				}
			}

			// Verify response message was sent
			select {
			case msg := <-client.send:
				var response Message
				if err := json.Unmarshal(msg, &response); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}

				expectedType := "subscribed"
				if !tt.subscribe {
					expectedType = "unsubscribed"
				}

				if response.Type != expectedType {
					t.Errorf("Response type mismatch: got %s, want %s", response.Type, expectedType)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("No response message sent")
			}
		})
	}
}

func TestClient_ReadPump(t *testing.T) {
	tests := []struct {
		name            string
		messages        []string
		initialSubs     map[string]bool
		expectedSubs    map[string]bool // Expected subscriptions after messages processed
		shouldError     bool
		closeConnection bool
	}{
		{
			name: "valid subscribe message",
			messages: []string{
				`{"type":"subscribe","symbols":["AAPL","GOOGL"]}`,
			},
			initialSubs:  map[string]bool{},
			expectedSubs: map[string]bool{"AAPL": true, "GOOGL": true},
			shouldError:  false,
		},
		{
			name: "valid unsubscribe message",
			messages: []string{
				`{"type":"unsubscribe","symbols":["AAPL"]}`,
			},
			initialSubs:  map[string]bool{"AAPL": true},
			expectedSubs: map[string]bool{},
			shouldError:  false,
		},
		{
			name: "invalid JSON message",
			messages: []string{
				`{invalid json}`,
			},
			initialSubs:  map[string]bool{},
			expectedSubs: map[string]bool{},
			shouldError:  false, // Should continue despite error
		},
		{
			name: "multiple messages",
			messages: []string{
				`{"type":"subscribe","symbols":["AAPL"]}`,
				`{"type":"subscribe","symbols":["GOOGL"]}`,
				`{"type":"unsubscribe","symbols":["AAPL"]}`,
			},
			initialSubs:  map[string]bool{},
			expectedSubs: map[string]bool{"GOOGL": true},
			shouldError:  false,
		},
		{
			name:            "cleanup on disconnect",
			messages:        []string{},
			initialSubs:     map[string]bool{"AAPL": true, "GOOGL": true},
			expectedSubs:    map[string]bool{}, // Should be cleared on disconnect
			shouldError:     false,
			closeConnection: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test WebSocket server
			testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Fatalf("Failed to upgrade connection: %v", err)
					return
				}
				defer conn.Close()

				// Send test messages
				for _, msg := range tt.messages {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
						t.Fatalf("Failed to write message: %v", err)
					}
				}

				if tt.closeConnection {
					// Give client time to process messages
					time.Sleep(50 * time.Millisecond)
					return
				}

				// Keep connection open for test duration
				time.Sleep(200 * time.Millisecond)
			}))
			defer testServer.Close()

			// Connect to test server
			wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("Failed to dial: %v", err)
			}

			// Create real server
			server := &Server{
				subscriptions: make(map[string]int),
				hub: &Hub{
					unregister:    make(chan *Client, 1),
					subscriptions: make(map[string]map[*Client]bool),
				},
			}

			client := &Client{
				conn:          conn,
				send:          make(chan []byte, 10),
				hub:           server.hub,
				subscriptions: copySubscriptions(tt.initialSubs),
				mu:            sync.RWMutex{},
			}

			// Run readPump in goroutine
			done := make(chan bool)
			go func() {
				client.readPump(server)
				done <- true
			}()

			// Wait for readPump to complete or timeout
			select {
			case <-done:
				// readPump completed
			case <-time.After(500 * time.Millisecond):
				// Force close connection to end test
				conn.Close()
				<-done
			}

			// Verify unregister was called
			select {
			case c := <-server.hub.unregister:
				if c != client {
					t.Error("Wrong client unregistered")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("Client was not unregistered")
			}
		})
	}
}

func TestClient_WritePump(t *testing.T) {
	receivedMessages := make([]string, 0)
	receivedTypes := make([]int, 0)
	var mu sync.Mutex

	// Create test WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		// Read messages from client
		done := make(chan bool)

		go func() {
			for {
				msgType, msg, err := conn.ReadMessage()
				if err != nil {
					done <- true
					return
				}
				mu.Lock()
				receivedTypes = append(receivedTypes, msgType)
				if msgType == websocket.TextMessage {
					receivedMessages = append(receivedMessages, string(msg))
				}
				mu.Unlock()
			}
		}()

		select {
		case <-done:
			// Connection closed
		case <-time.After(1 * time.Second):
			// Timeout
		}
	}))
	defer server.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 10),
	}

	// Send test messages
	testMessages := []string{
		`{"type":"test","data":"message1"}`,
		`{"type":"test","data":"message2"}`,
	}

	// Run writePump in goroutine
	done := make(chan bool)
	go func() {
		client.writePump()
		done <- true
	}()

	// Send messages
	for _, msg := range testMessages {
		client.send <- []byte(msg)
	}

	// Wait a bit for messages to be processed
	time.Sleep(200 * time.Millisecond)

	// Close send channel to trigger writePump exit
	close(client.send)

	// Wait for writePump to complete
	select {
	case <-done:
		// writePump completed successfully
	case <-time.After(1 * time.Second):
		t.Error("writePump did not complete in time")
	}

	// Verify we received the test messages
	mu.Lock()
	defer mu.Unlock()

	if len(receivedMessages) < len(testMessages) {
		t.Errorf("Expected at least %d messages, got %d", len(testMessages), len(receivedMessages))
	}

	// Verify we got text messages
	hasTextMessage := false
	for _, msgType := range receivedTypes {
		if msgType == websocket.TextMessage {
			hasTextMessage = true
			break
		}
	}

	if !hasTextMessage {
		t.Error("No text messages received")
	}
}

func TestClient_ReadPump_CleanupOnDisconnect(t *testing.T) {
	// Create test WebSocket server that immediately closes
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Immediately close to trigger cleanup
		conn.Close()
	}))
	defer testServer.Close()

	// Connect to test server
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	// Create server with initial subscriptions
	server := &Server{
		subscriptions: map[string]int{"AAPL": 1, "GOOGL": 1},
		hub: &Hub{
			unregister:    make(chan *Client, 1),
			subscriptions: make(map[string]map[*Client]bool),
		},
	}

	client := &Client{
		conn:          conn,
		send:          make(chan []byte, 10),
		hub:           server.hub,
		subscriptions: map[string]bool{"AAPL": true, "GOOGL": true},
		mu:            sync.RWMutex{},
	}

	// Add client to hub subscriptions
	for symbol := range client.subscriptions {
		if server.hub.subscriptions[symbol] == nil {
			server.hub.subscriptions[symbol] = make(map[*Client]bool)
		}
		server.hub.subscriptions[symbol][client] = true
	}

	// Run readPump
	done := make(chan bool)
	go func() {
		client.readPump(server)
		done <- true
	}()

	// Wait for cleanup
	select {
	case <-done:
		// Good - readPump exited
	case <-time.After(1 * time.Second):
		t.Fatal("readPump did not complete cleanup")
	}

	// Verify cleanup was called
	select {
	case <-server.hub.unregister:
		// Good - client was unregistered
	case <-time.After(100 * time.Millisecond):
		t.Error("Client was not unregistered during cleanup")
	}

	// Verify server subscriptions were decremented
	server.mu.Lock()
	defer server.mu.Unlock()

	// After cleanup, subscriptions should be decremented
	for symbol := range client.subscriptions {
		if server.subscriptions[symbol] != 0 {
			t.Errorf("Server subscription for %s not cleaned up: got %d, want 0",
				symbol, server.subscriptions[symbol])
		}
	}
}

// Helper function to copy subscriptions map
func copySubscriptions(m map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for k, v := range m {
		result[k] = v
	}
	return result
}
