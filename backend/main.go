package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/NotVinay/stock-dashboard/backend/config"
	"github.com/NotVinay/stock-dashboard/backend/finnhub"
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
	// Load configuration.
	cfg := config.GetAppConfig()
	
	// Create Finnhub client and API handler.
	finnhubClient := finnhub.NewClient(cfg.FinnhubAPIURL, cfg.FinnhubAPIKey)
	api := newAPIHandler(finnhubClient)
	
	// Setup HTTP routes.
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
	
	// Start server.
	port := cfg.Port
	fmt.Printf("🚀 Stock Dashboard API starting on port %s\n", port)
	fmt.Printf("📊 Environment: %s\n", cfg.Environment)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}