package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/NotVinay/stock-dashboard/backend/config"
)

// Response structure for API responses
type Response struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

// enableCORS middleware to handle CORS for Angular frontend
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next(w, r)
	}
}

// healthCheck handler
func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := Response{
		Message: "Stock Dashboard API is running",
		Status:  "healthy",
	}
	json.NewEncoder(w).Encode(response)
}

func main() {
	cfg := config.GetAppConfig()

	// Setup routes
	http.HandleFunc("/", enableCORS(healthCheck))
	http.HandleFunc("/api/health", enableCORS(healthCheck))

	// Start server
	fmt.Printf("Server starting on port %s...\n", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}