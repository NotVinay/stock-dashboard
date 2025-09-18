package websocket

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestHub_ClientRegistration(t *testing.T) {
	tests := []struct {
		name            string
		numClients      int
		expectedClients int
	}{
		{
			name:            "register single client",
			numClients:      1,
			expectedClients: 1,
		},
		{
			name:            "register multiple clients",
			numClients:      5,
			expectedClients: 5,
		},
		{
			name:            "register no clients",
			numClients:      0,
			expectedClients: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newHub()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start hub
			go hub.run(ctx)

			clients := make([]*Client, tt.numClients)
			for i := 0; i < tt.numClients; i++ {
				clients[i] = &Client{
					send:          make(chan []byte, 256),
					subscriptions: make(map[string]bool),
				}
				hub.register <- clients[i]
			}

			// Wait for registration to complete
			time.Sleep(50 * time.Millisecond)

			hub.mu.RLock()
			actualClients := len(hub.clients)
			hub.mu.RUnlock()

			if actualClients != tt.expectedClients {
				t.Errorf("Client count mismatch: got %d, want %d", actualClients, tt.expectedClients)
			}
		})
	}
}

func TestHub_ClientUnregistration(t *testing.T) {
	tests := []struct {
		name                string
		initialClients      int
		unregisterClients   int
		clientSubscriptions map[int][]string // client index -> symbols
		expectedClients     int
		expectedSubs        map[string]int // symbol -> subscriber count
	}{
		{
			name:              "unregister single client",
			initialClients:    3,
			unregisterClients: 1,
			clientSubscriptions: map[int][]string{
				0: {"AAPL", "GOOGL"},
				1: {"AAPL"},
				2: {"MSFT"},
			},
			expectedClients: 2,
			expectedSubs: map[string]int{
				"AAPL": 1,
				"MSFT": 1,
			},
		},
		{
			name:              "unregister all clients",
			initialClients:    3,
			unregisterClients: 3,
			clientSubscriptions: map[int][]string{
				0: {"AAPL"},
				1: {"GOOGL"},
				2: {"MSFT"},
			},
			expectedClients: 0,
			expectedSubs:    map[string]int{},
		},
		{
			name:              "unregister client with no subscriptions",
			initialClients:    2,
			unregisterClients: 1,
			clientSubscriptions: map[int][]string{
				1: {"AAPL"},
			},
			expectedClients: 1,
			expectedSubs: map[string]int{
				"AAPL": 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newHub()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start hub
			go hub.run(ctx)

			// Register clients and set up subscriptions
			clients := make([]*Client, tt.initialClients)
			for i := 0; i < tt.initialClients; i++ {
				clients[i] = &Client{
					send:          make(chan []byte, 256),
					subscriptions: make(map[string]bool),
				}

				// Set up subscriptions for this client
				if symbols, ok := tt.clientSubscriptions[i]; ok {
					for _, symbol := range symbols {
						clients[i].subscriptions[symbol] = true
						hub.addSubscription(symbol, clients[i])
					}
				}

				hub.register <- clients[i]
			}

			// Wait for registration to complete
			time.Sleep(50 * time.Millisecond)

			// Unregister specified clients
			for i := 0; i < tt.unregisterClients; i++ {
				hub.unregister <- clients[i]
			}

			// Wait for unregistration to complete
			time.Sleep(50 * time.Millisecond)

			// Verify client count
			hub.mu.RLock()
			actualClients := len(hub.clients)
			actualSubs := make(map[string]int)
			for symbol, subs := range hub.subscriptions {
				actualSubs[symbol] = len(subs)
			}
			hub.mu.RUnlock()

			if actualClients != tt.expectedClients {
				t.Errorf("Client count mismatch: got %d, want %d", actualClients, tt.expectedClients)
			}

			if diff := cmp.Diff(tt.expectedSubs, actualSubs); diff != "" {
				t.Errorf("Subscription count mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHub_Subscriptions(t *testing.T) {
	tests := []struct {
		name         string
		operations   []subscriptionOp
		expectedSubs map[string][]string // symbol -> client IDs
	}{
		{
			name: "add single subscription",
			operations: []subscriptionOp{
				{action: "add", symbol: "AAPL", clientID: "c1"},
			},
			expectedSubs: map[string][]string{
				"AAPL": {"c1"},
			},
		},
		{
			name: "add multiple subscriptions same symbol",
			operations: []subscriptionOp{
				{action: "add", symbol: "AAPL", clientID: "c1"},
				{action: "add", symbol: "AAPL", clientID: "c2"},
				{action: "add", symbol: "AAPL", clientID: "c3"},
			},
			expectedSubs: map[string][]string{
				"AAPL": {"c1", "c2", "c3"},
			},
		},
		{
			name: "add and remove subscriptions",
			operations: []subscriptionOp{
				{action: "add", symbol: "AAPL", clientID: "c1"},
				{action: "add", symbol: "AAPL", clientID: "c2"},
				{action: "add", symbol: "GOOGL", clientID: "c1"},
				{action: "remove", symbol: "AAPL", clientID: "c1"},
			},
			expectedSubs: map[string][]string{
				"AAPL":  {"c2"},
				"GOOGL": {"c1"},
			},
		},
		{
			name: "remove all subscriptions for symbol",
			operations: []subscriptionOp{
				{action: "add", symbol: "AAPL", clientID: "c1"},
				{action: "add", symbol: "AAPL", clientID: "c2"},
				{action: "remove", symbol: "AAPL", clientID: "c1"},
				{action: "remove", symbol: "AAPL", clientID: "c2"},
			},
			expectedSubs: map[string][]string{},
		},
		{
			name: "remove non-existent subscription",
			operations: []subscriptionOp{
				{action: "add", symbol: "AAPL", clientID: "c1"},
				{action: "remove", symbol: "GOOGL", clientID: "c1"},
				{action: "remove", symbol: "AAPL", clientID: "c2"},
			},
			expectedSubs: map[string][]string{
				"AAPL": {"c1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := newHub()
			clientMap := make(map[string]*Client)

			// Execute operations
			for _, op := range tt.operations {
				// Create client if it doesn't exist
				if _, exists := clientMap[op.clientID]; !exists {
					clientMap[op.clientID] = &Client{
						send:          make(chan []byte, 256),
						subscriptions: make(map[string]bool),
					}
				}

				client := clientMap[op.clientID]
				switch op.action {
				case "add":
					hub.addSubscription(op.symbol, client)
				case "remove":
					hub.removeSubscription(op.symbol, client)
				}
			}

			// Verify subscriptions
			actualSubs := make(map[string][]string)
			hub.mu.RLock()
			for symbol, clients := range hub.subscriptions {
				for client := range clients {
					// Find client ID
					for id, c := range clientMap {
						if c == client {
							actualSubs[symbol] = append(actualSubs[symbol], id)
							break
						}
					}
				}
			}
			hub.mu.RUnlock()

			// Sort client IDs for consistent comparison
			for symbol := range actualSubs {
				sortStrings(actualSubs[symbol])
			}
			for symbol := range tt.expectedSubs {
				sortStrings(tt.expectedSubs[symbol])
			}

			if diff := cmp.Diff(tt.expectedSubs, actualSubs); diff != "" {
				t.Errorf("Subscriptions mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start hub
	go hub.run(ctx)

	// Create and register multiple clients
	numClients := 3
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
		}
		hub.register <- clients[i]
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Broadcast message
	testMessage := []byte(`{"type":"test","data":"broadcast"}`)
	hub.broadcast <- testMessage

	// Verify all clients received the message
	for i, client := range clients {
		select {
		case msg := <-client.send:
			if string(msg) != string(testMessage) {
				t.Errorf("Client %d received wrong message: got %s, want %s", i, string(msg), string(testMessage))
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Client %d did not receive broadcast message", i)
		}
	}
}

func TestHub_ContextCancellation(t *testing.T) {
	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())

	// Create and register clients
	numClients := 3
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			send:          make(chan []byte, 256),
			subscriptions: make(map[string]bool),
		}
	}

	// Start hub
	hubDone := make(chan bool)
	go func() {
		hub.run(ctx)
		hubDone <- true
	}()

	// Register clients
	for i := 0; i < numClients; i++ {
		hub.register <- clients[i]
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for hub to stop
	select {
	case <-hubDone:
		// Hub stopped successfully
	case <-time.After(100 * time.Millisecond):
		t.Error("Hub did not stop after context cancellation")
	}

	// Verify all client channels are closed
	for i, client := range clients {
		select {
		case _, ok := <-client.send:
			if ok {
				t.Errorf("Client %d channel not closed", i)
			}
		default:
			// Channel is closed or empty
		}
	}

	// Verify clients map is empty
	hub.mu.RLock()
	if len(hub.clients) != 0 {
		t.Errorf("Clients not cleaned up: %d remaining", len(hub.clients))
	}
	hub.mu.RUnlock()
}

func TestHub_ConcurrentOperations(t *testing.T) {
	hub := newHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start hub
	go hub.run(ctx)

	// Perform concurrent operations
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			client := &Client{
				send:          make(chan []byte, 256),
				subscriptions: make(map[string]bool),
			}

			for j := 0; j < numOperations; j++ {
				// Mix of operations
				switch j % 4 {
				case 0:
					hub.register <- client
				case 1:
					hub.addSubscription("SYMBOL", client)
				case 2:
					hub.removeSubscription("SYMBOL", client)
				case 3:
					select {
					case hub.broadcast <- []byte("test"):
					default:
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify hub is still functional
	testClient := &Client{
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]bool),
	}
	hub.register <- testClient

	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	if _, ok := hub.clients[testClient]; !ok {
		t.Error("Hub not functional after concurrent operations")
	}
	hub.mu.RUnlock()
}

// Helper types and functions

type subscriptionOp struct {
	action   string // "add" or "remove"
	symbol   string
	clientID string
}

func sortStrings(s []string) {
	// Simple bubble sort for test purposes
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
