package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NotVinay/stock-dashboard/backend/finnhub"
	"github.com/google/go-cmp/cmp"
)

// tFatalf is a helper to print file and line number on failure.
func tFatalf(t *testing.T, format string, args ...interface{}) {
	t.Helper()
	t.Fatalf(format, args...)
}

// setupTestServer sets up a test HTTP server along with a Finnhub client that is configured to talk to that server.
// It returns the mux to add handlers to, and a teardown function.
func setupTestServerAndHandler(t *testing.T) (*apiHandler, *http.ServeMux, func()) {
	t.Helper()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	// Create a client pointing to the test server
	client := finnhub.NewClient(server.URL, "test-key")
	// Create the handler instance
	handler := newAPIHandler(client)

	return handler, mux, func() {
		server.Close()
	}
}

func TestHandleIndices(t *testing.T) {
	// Note: The variable `handler` is now available.
	handler, mux, teardown := setupTestServerAndHandler(t)
	defer teardown()

	// Mock Finnhub API for quotes
	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		// Add a way to trigger an error from the mock
		if r.URL.Query().Get("symbol") == "^GSPC" { // The first index symbol
			// Simulate an error for one of the API calls
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		quote := finnhub.Quote{Current: 100, Change: 1, ChangePercent: 1}
		json.NewEncoder(w).Encode(quote)
	})

	testCases := []struct {
		name          string
		method        string
		path          string
		wantStatus    int
		wantBody      string
		checkBodyJSON func(t *testing.T, body []byte)
	}{
		{
			name:       "Success",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/indices",
			wantStatus: http.StatusOK,
			checkBodyJSON: func(t *testing.T, body []byte) {
				var indices []finnhub.StockIndex
				if err := json.Unmarshal(body, &indices); err != nil {
					t.Fatalf("could not decode response: %v", err)
				}
				if len(indices) != 8 {
					t.Errorf("expected 8 indices, got %d", len(indices))
				}
				// The first one (^GSPC) will fail, the rest should succeed.
				if indices[0].CurrentPrice != 0 {
					t.Errorf("expected first index to have 0 price due to error, got %f", indices[0].CurrentPrice)
				}
				if indices[1].CurrentPrice != 100 {
					t.Errorf("expected current price to be 100, got %f", indices[1].CurrentPrice)
				}
			},
		},
		{
			name:       "Method Not Allowed",
			method:     http.MethodPost,
			path:       "/api/v1/stocks/indices",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `{"error":"Method Not Allowed","message":"Method not allowed"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.handleIndices(rr, req)

			if status := rr.Code; status != tc.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.wantStatus)
			}

			if tc.checkBodyJSON != nil {
				tc.checkBodyJSON(t, rr.Body.Bytes())
			} else if tc.wantBody != "" {
				if diff := cmp.Diff(tc.wantBody, strings.TrimSpace(rr.Body.String())); diff != "" {
					t.Errorf("handler returned unexpected body: (-want +got)\n%s", diff)
				}
			}
		})
	}
}

func TestHandleExchangeStocks(t *testing.T) {
	handler, mux, teardown := setupTestServerAndHandler(t)
	defer teardown()

	mockStocks := []finnhub.Stock{
		{Symbol: "AAPL", Description: "APPLE INC"},
		{Symbol: "GOOG", Description: "ALPHABET INC-CL A"},
	}
	mockQuote := finnhub.Quote{Current: 150.0, Change: 1.5, ChangePercent: 1.0}

	mux.HandleFunc("/stock/symbol", func(w http.ResponseWriter, r *http.Request) {
		exchange := r.URL.Query().Get("exchange")
		if exchange == "FAIL" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if exchange != "US" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(mockStocks)
	})

	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mockQuote)
	})

	testCases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "Success",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/exchange/US",
			wantStatus: http.StatusOK,
			wantBody:   `[{"symbol":"AAPL","description":"APPLE INC","displaySymbol":"","type":"","price":150,"change":1.5,"change_percent":1},{"symbol":"GOOG","description":"ALPHABET INC-CL A","displaySymbol":"","type":"","price":150,"change":1.5,"change_percent":1}]`,
		},
		{
			name:       "Bad Request - No Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/exchange/",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"Bad Request","message":"Exchange symbol is required"}`,
		},
		{
			name:       "Internal Server Error from Finnhub - Invalid Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/exchange/FAIL",
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"Internal Server Error","message":"Failed to fetch stocks"}`,
		},
		{
			name:       "Method Not Allowed",
			method:     http.MethodPost,
			path:       "/api/v1/stocks/exchange/US",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `{"error":"Method Not Allowed","message":"Method not allowed"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.handleExchangeStocks(rr, req)

			if status := rr.Code; status != tc.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.wantStatus)
			}

			if diff := cmp.Diff(tc.wantBody, strings.TrimSpace(rr.Body.String())); diff != "" {
				t.Errorf("handler returned unexpected body: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestHandleStockHistory(t *testing.T) {
	handler, mux, teardown := setupTestServerAndHandler(t)
	defer teardown()


	
	currentTime := time.Now()
    prevDay := currentTime.AddDate(0, 0, -1)

	mockCandle := finnhub.Candle{
		Open:       []float64{100, 102},
		High:       []float64{103, 104},
		Low:        []float64{99, 101},
		Close:      []float64{102, 103},
		Volume:     []int64{1000, 1200},
		Timestamp:  []int64{prevDay.Unix(), currentTime.Unix()},
		Status:     "ok",
	}

	mux.HandleFunc("/stock/candle", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "FAIL" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(mockCandle)
	})

	testCases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "Success",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/history/AAPL",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Bad Request - No Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/history/",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"Bad Request","message":"Stock symbol is required"}`,
		},
		{
			name:       "Internal Server Error from Finnhub - Invalid Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/history/FAIL",
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"Internal Server Error","message":"Failed to fetch historical data"}`,
		},
		{
			name:       "Method Not Allowed",
			method:     http.MethodPost,
			path:       "/api/v1/stocks/history/AAPL",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `{"error":"Method Not Allowed","message":"Method not allowed"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.handleStockHistory(rr, req)

			if status := rr.Code; status != tc.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				var result finnhub.HistoricalData
				if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
					tFatalf(t, "could not decode response: %v", err)
				}
				expectedData := finnhub.HistoricalData{
					Symbol: "AAPL",
					Data: []finnhub.DataPoint{
						{Timestamp: prevDay.Unix(), Date: prevDay.Format("2006-01-02"), Open: 100, High: 103, Low: 99, Close: 102, Volume: 1000},
						{Timestamp: currentTime.Unix(), Date: currentTime.Format("2006-01-02"), Open: 102, High: 104, Low: 101, Close: 103, Volume: 1200},
					},
				}
				// The handler converts Candle to HistoricalData, so we compare against that.
				// The symbol is extracted from the path.
				if diff := cmp.Diff(expectedData, result); diff != "" {
					t.Errorf("handleStockHistory() returned unexpected data: (-want +got)\n%s", diff)
				}
			} else if diff := cmp.Diff(tc.wantBody, strings.TrimSpace(rr.Body.String())); diff != "" {
				t.Errorf("handler returned unexpected body: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestHandleStockQuote(t *testing.T) {
	handler, mux, teardown := setupTestServerAndHandler(t)
	defer teardown()

	mockQuote := finnhub.Quote{Current: 150.0, Change: 1.5, ChangePercent: 1.0}

	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "FAIL" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(mockQuote)
	})

	testCases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "Success",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/quote/AAPL",
			wantStatus: http.StatusOK,
			wantBody:   `{"c":150,"d":1.5,"dp":1,"h":0,"l":0,"o":0,"pc":0,"t":0}`,
		},
		{
			name:       "Bad Request - No Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/quote/",
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"Bad Request","message":"Stock symbol is required"}`,
		},
		{
			name:       "Internal Server Error from Finnhub - Invalid Symbol",
			method:     http.MethodGet,
			path:       "/api/v1/stocks/quote/FAIL",
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"Internal Server Error","message":"Failed to fetch quote"}`,
		},
		{
			name:       "Method Not Allowed",
			method:     http.MethodPost,
			path:       "/api/v1/stocks/quote/AAPL",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `{"error":"Method Not Allowed","message":"Method not allowed"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			handler.handleStockQuote(rr, req)

			if status := rr.Code; status != tc.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.wantStatus)
			}

			if diff := cmp.Diff(tc.wantBody, strings.TrimSpace(rr.Body.String())); diff != "" {
				t.Errorf("handler returned unexpected body: (-want +got)\n%s", diff)
			}
		})
	}
}