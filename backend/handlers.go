package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
	"github.com/NotVinay/stock-dashboard/backend/finnhub"
)

// apiHandler holds dependencies for HTTP handlers.
type apiHandler struct {
	client *finnhub.Client
}

// newAPIHandler creates a new apiHandler with its dependencies.
func newAPIHandler(client *finnhub.Client) *apiHandler {
	return &apiHandler{client: client}
}

// ErrorResponse represents an error API response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// sendJSON sends a JSON response.
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

// sendError sends an error response.
func sendError(w http.ResponseWriter, status int, message string) {
	sendJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// handleIndices handles GET /api/v1/stocks/indices
// Gets popular stock indices.
func (h *apiHandler) handleIndices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	indices, err := h.client.GetPopularIndices()
	if err != nil {
		log.Printf("Error fetching indices: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to fetch indices")
		return
	}

	sendJSON(w, http.StatusOK, indices)
}

// handleExchangeStocks handles GET /api/v1/stocks/exchange/{symbol}
// Fetches stocks associated with given exchange symbol.
func (h *apiHandler) handleExchangeStocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract exchange symbol from URL path.
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/stocks/exchange/")
	if path == "" {
		sendError(w, http.StatusBadRequest, "Exchange symbol is required")
		return
	}

	exchange := strings.ToUpper(path)

	stocks, err := h.client.GetStocksByExchange(exchange)
	if err != nil {
		log.Printf("Error fetching stocks for exchange %s: %v", exchange, err)
		sendError(w, http.StatusInternalServerError, "Failed to fetch stocks")
		return
	}

	sendJSON(w, http.StatusOK, stocks)
}

// handleStockHistory handles GET /api/v1/stocks/history/{symbol}
// Gets stock history for given stock symbol.
func (h *apiHandler) handleStockHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract stock symbol from URL path.
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/stocks/history/")
	symbol := strings.ToUpper(path)
	if symbol == "" {
		sendError(w, http.StatusBadRequest, "Stock symbol is required")
		return
	}

	// Get query parameters for time range and resolution.
	query := r.URL.Query()
	resolution := query.Get("resolution")
	if resolution == "" {
		resolution = "D" // Default to daily
	}

	// Parse time range.
	var from, to int64
	if fromStr := query.Get("from"); fromStr != "" {
		if fromTime, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = fromTime.Unix()
		}
	}
	if toStr := query.Get("to"); toStr != "" {
		if toTime, err := time.Parse("2006-01-02", toStr); err == nil {
			to = toTime.Unix()
		}
	}

	// Default time range if not specified.
	if from == 0 {
		from = time.Now().AddDate(0, -3, 0).Unix() // 3 months ago
	}
	if to == 0 {
		to = time.Now().Unix()
	}

	historicalData, err := h.client.GetHistoricalData(symbol, resolution, from, to)
	if err != nil {
		log.Printf("Error fetching historical data for %s: %v", symbol, err)
		sendError(w, http.StatusInternalServerError, "Failed to fetch historical data")
		return
	}

	sendJSON(w, http.StatusOK, historicalData)
}

// handleStockQuote handles GET /api/v1/stocks/quote/{symbol}
// Gets quote for given stock symbol.
func (h *apiHandler) handleStockQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract stock symbol from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/stocks/quote/")
	symbol := strings.ToUpper(path)
	if symbol == "" {
		sendError(w, http.StatusBadRequest, "Stock symbol is required")
		return
	}

	quote, err := h.client.GetQuote(symbol)
	if err != nil {
		log.Printf("Error fetching quote for %s: %v", symbol, err)
		sendError(w, http.StatusInternalServerError, "Failed to fetch quote")
		return
	}

	sendJSON(w, http.StatusOK, quote)
}