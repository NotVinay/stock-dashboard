package finnhub

import (
	"encoding/json"
	"github.com/google/go-cmp/cmp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// setup creates a test client, a mock server, and a teardown function.
func setup(t *testing.T) (*Client, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	client := NewClient(server.URL, "test-api-key")

	teardown := func() {
		server.Close()
	}
	return client, mux, teardown
}

func TestNewClient(t *testing.T) {
	apiKey := "my-secret-key"
	testURL := "http://test.url/api"
	client := NewClient(testURL, apiKey)

	if client.apiKey != apiKey {
		t.Errorf("expected apiKey to be %q, got %q", apiKey, client.apiKey)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be initialized, but it was nil")
	}
	if client.baseURL != testURL {
		t.Errorf("expected baseURL to be %q, got %q", testURL, client.baseURL)
	}
}

func TestClient_GetPopularIndices(t *testing.T) {
	client, mux, teardown := setup(t)
	defer teardown()

	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		var quote Quote
		switch symbol {
		case "^GSPC":
			quote = Quote{Current: 4500, Change: 20, ChangePercent: 0.44}
		case "^DJI":
			quote = Quote{Current: 35000, Change: -100, ChangePercent: -0.28}
		default:
			// For other indices, return a quote to ensure they are processed
			quote = Quote{Current: 100, Change: 1, ChangePercent: 1}
		}
		json.NewEncoder(w).Encode(quote)
	})

	expectedIndices := []StockIndex{
		{Symbol: "^GSPC", Name: "S&P 500", Country: "US", Currency: "USD", Exchange: "INDEX", CurrentPrice: 4500, Change: 20, ChangePercent: 0.44},
		{Symbol: "^DJI", Name: "Dow Jones Industrial Average", Country: "US", Currency: "USD", Exchange: "INDEX", CurrentPrice: 35000, Change: -100, ChangePercent: -0.28},
		{Symbol: "^IXIC", Name: "NASDAQ Composite", Country: "US", Currency: "USD", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
		{Symbol: "^FTSE", Name: "FTSE 100", Country: "UK", Currency: "GBP", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
		{Symbol: "^N225", Name: "Nikkei 225", Country: "JP", Currency: "JPY", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
		{Symbol: "^HSI", Name: "Hang Seng Index", Country: "HK", Currency: "HKD", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
		{Symbol: "^GDAXI", Name: "DAX", Country: "DE", Currency: "EUR", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
		{Symbol: "^FCHI", Name: "CAC 40", Country: "FR", Currency: "EUR", Exchange: "INDEX", CurrentPrice: 100, Change: 1, ChangePercent: 1},
	}

	indices, err := client.GetPopularIndices()
	if err != nil {
		t.Fatalf("GetPopularIndices() returned an unexpected error: %v", err)
	}

	if diff := cmp.Diff(expectedIndices, indices); diff != "" {
		t.Errorf("GetPopularIndices() mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_GetStocksByExchange(t *testing.T) {
	client, mux, teardown := setup(t)
	defer teardown()

	// Mock response for stock symbols
	mockStocks := []Stock{
		{Symbol: "AAPL", Description: "APPLE INC"},
		{Symbol: "GOOG", Description: "ALPHABET INC-CL A"},
	}

	// Mock response for quotes
	mockQuote := Quote{Current: 150.0, Change: 1.5, ChangePercent: 1.0}

	mux.HandleFunc("/stock/symbol", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("exchange") != "US" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(mockStocks)
	})

	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mockQuote)
	})

	expectedStocks := []Stock{
		{Symbol: "AAPL", Description: "APPLE INC", Price: 150.0, Change: 1.5, ChangePercent: 1.0},
		{Symbol: "GOOG", Description: "ALPHABET INC-CL A", Price: 150.0, Change: 1.5, ChangePercent: 1.0},
	}

	stocks, err := client.GetStocksByExchange("US")
	if err != nil {
		t.Fatalf("GetStocksByExchange() returned an unexpected error: %v", err)
	}

	// The function limits to 50 stocks, and our mock returns 2.
	// It then fetches quotes for up to 10 stocks.
	if diff := cmp.Diff(expectedStocks, stocks); diff != "" {
		t.Errorf("GetStocksByExchange() mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_GetQuote(t *testing.T) {
	client, mux, teardown := setup(t)
	defer teardown()

	expectedQuote := &Quote{
		Current:       285.9,
		Change:        2.3,
		ChangePercent: 0.81,
		High:          286.0,
		Low:           283.5,
		Open:          284.0,
		PreviousClose: 283.6,
	}

	mux.HandleFunc("/quote", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("symbol") != "AAPL" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("token") != "test-api-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedQuote)
	})

	testCases := []struct {
		name        string
		symbol      string
		want        *Quote
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:   "Success",
			symbol: "AAPL",
			want:   expectedQuote,
		},
		{
			name:       "API Error",
			symbol:     "FAIL",
			wantErr:    true,
			wantErrMsg: "API error: not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := client.GetQuote(tc.symbol)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("expected error message to contain %q, got %q", tc.wantErrMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.want, quote); diff != "" {
				t.Errorf("GetQuote() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_GetHistoricalData(t *testing.T) {
	client, mux, teardown := setup(t)
	defer teardown()

	mockCandle := Candle{
		Close:     []float64{284.9, 285.2},
		High:      []float64{286.1, 285.5},
		Low:       []float64{283.0, 284.0},
		Open:      []float64{284.0, 284.5},
		Status:    "ok",
		Timestamp: []int64{1572540000, 1572626400},
		Volume:    []int64{100000, 120000},
	}

	mux.HandleFunc("/stock/candle", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "NO_DATA" {
			json.NewEncoder(w).Encode(Candle{Status: "no_data"})
			return
		}
		if symbol != "AAPL" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(mockCandle)
	})

	testCases := []struct {
		name       string
		symbol     string
		want	   *HistoricalData
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "Success",
			symbol:     "AAPL",
			want: &HistoricalData{
				Symbol: "AAPL",
				Data: []DataPoint{
					{Timestamp: 1572540000, Date: "2019-10-31", Open: 284.0, High: 286.1, Low: 283.0, Close: 284.9, Volume: 100000},
					{Timestamp: 1572626400, Date: "2019-11-01", Open: 284.5, High: 285.5, Low: 284.0, Close: 285.2, Volume: 120000},
				},
			},
		},
		{
			name:       "No Data",
			symbol:     "NO_DATA",
			wantErr:    true,
			wantErrMsg: "no data available for symbol NO_DATA",
		},
		{
			name:       "API Error",
			symbol:     "UNKNOWN",
			wantErr:    true,
			wantErrMsg: "API error: not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			from := time.Now().Add(-24 * time.Hour).Unix()
			to := time.Now().Unix()

			data, err := client.GetHistoricalData(tc.symbol, "D", from, to)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("expected error message to contain %q, got %q", tc.wantErrMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.want, data); diff != "" {
				t.Errorf("GetHistoricalData() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
