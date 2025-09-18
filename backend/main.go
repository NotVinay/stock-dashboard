package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NotVinay/stock-dashboard/backend/config"
	"github.com/NotVinay/stock-dashboard/backend/finnhub"
	"github.com/NotVinay/stock-dashboard/backend/websocket"
)

// CORS middleware.
func enableCORS(cfg *config.Config, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false

		// Check if origin is in allowed list.
		for _, allowedOrigin := range cfg.AllowedOrigins {
			if origin == allowedOrigin || allowedOrigin == "*" {
				allowed = true
				break
			}
		}

		if allowed || cfg.Environment == "development" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func main() {
	// Load config.
	cfg := config.GetAppConfig()
	finnhubClient := finnhub.NewClient(cfg.FinnhubAPIURL, cfg.FinnhubAPIKey)
	api := newAPIHandler(finnhubClient)
	wsURL := finnhubClient.GetWebSocketURL()
	wsServer := websocket.NewServer(wsURL)

	// --- SETUP GRACEFUL SHUTDOWN ---
	// Create a context that can be cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a channel to listen for OS signals.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// --- START WEBSOCKET SERVER ---
	if err := wsServer.Start(ctx); err != nil {
		log.Fatalf("Failed to start WebSocket server: %v", err)
	}

	// --- SETUP HTTP ROUTES ---
	http.HandleFunc("/api/health", enableCORS(cfg, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy","message":"Stock Dashboard API is running"}`))
	}))
	// Stock API endpoints.
	http.HandleFunc("/api/v1/stocks/indices", enableCORS(cfg, api.handleIndices))
	http.HandleFunc("/api/v1/stocks/exchange/", enableCORS(cfg, func(w http.ResponseWriter, r *http.Request) {
		api.handleExchangeStocks(w, r)
	}))
	http.HandleFunc("/api/v1/stocks/history/", enableCORS(cfg, func(w http.ResponseWriter, r *http.Request) {
		api.handleStockHistory(w, r)
	}))
	http.HandleFunc("/api/v1/stocks/quote/", enableCORS(cfg, func(w http.ResponseWriter, r *http.Request) {
		api.handleStockQuote(w, r)
	}))
	http.HandleFunc("/ws", wsServer.HandleConnection) // Simplified handler call

	// --- START HTTP SERVER IN A GOROUTINE ---
	httpServer := &http.Server{Addr: ":" + cfg.Port}
	go func() {
		fmt.Printf("🚀 Stock Dashboard API starting on port %s\n", cfg.Port)
		fmt.Printf("📊 Environment: %s\n", cfg.Environment)
		fmt.Printf("🔌 WebSocket endpoint: ws://localhost:%s/ws\n", cfg.Port)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// --- BLOCK AND WAIT FOR SHUTDOWN SIGNAL ---
	<-signalChan
	log.Println("Shutdown signal received.")

	// --- INITIATE SHUTDOWN ---
	// Create a shutdown context with a timeout.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shut down the WebSocket server.
	wsServer.Shutdown()

	// Shut down the HTTP server.
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("All services stopped.")
}
