package finnhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a Finnhub API client
type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Finnhub API client
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: apiURL,
	}
}

// StockIndex represents a stock market index.
type StockIndex struct {
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	Country     string  `json:"country"`
	Currency    string  `json:"currency"`
	Exchange    string  `json:"exchange"`
	CurrentPrice float64 `json:"current_price,omitempty"`
	Change      float64 `json:"change,omitempty"`
	ChangePercent float64 `json:"change_percent,omitempty"`
}

// Stock represents a stock.
type Stock struct {
	Symbol      string  `json:"symbol"`
	Description string  `json:"description"`
	DisplaySymbol string `json:"displaySymbol"`
	Type        string  `json:"type"`
	Currency    string  `json:"currency,omitempty"`
	Price       float64 `json:"price,omitempty"`
	Change      float64 `json:"change,omitempty"`
	ChangePercent float64 `json:"change_percent,omitempty"`
}

// Quote represents current stock quote received from Finnhub API.
type Quote struct {
	Current        float64 `json:"c"`
	Change         float64 `json:"d"`
	ChangePercent  float64 `json:"dp"`
	High           float64 `json:"h"`
	Low            float64 `json:"l"`
	Open           float64 `json:"o"`
	PreviousClose  float64 `json:"pc"`
	Timestamp      int64   `json:"t"`
}

// Candle represents historical price data received from Finnhub API.
type Candle struct {
	Close     []float64 `json:"c"`
	High      []float64 `json:"h"`
	Low       []float64 `json:"l"`
	Open      []float64 `json:"o"`
	Status    string    `json:"s"`
	Timestamp []int64   `json:"t"`
	Volume    []int64   `json:"v"`
}

// HistoricalData represents formatted historical data.
type HistoricalData struct {
	Symbol string      `json:"symbol"`
	Data   []DataPoint `json:"data"`
}

// DataPoint represents a single historical data point.
type DataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Date      string  `json:"date"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

// GetPopularIndices returns a list of popular stock indices.
func (c *Client) GetPopularIndices() ([]StockIndex, error) {
	// Hardcoded popular indices as Finnhub doesn't have a direct endpoint for this.
	indices := []StockIndex{
		{Symbol: "^GSPC", Name: "S&P 500", Country: "US", Currency: "USD", Exchange: "INDEX"},
		{Symbol: "^DJI", Name: "Dow Jones Industrial Average", Country: "US", Currency: "USD", Exchange: "INDEX"},
		{Symbol: "^IXIC", Name: "NASDAQ Composite", Country: "US", Currency: "USD", Exchange: "INDEX"},
		{Symbol: "^FTSE", Name: "FTSE 100", Country: "UK", Currency: "GBP", Exchange: "INDEX"},
		{Symbol: "^N225", Name: "Nikkei 225", Country: "JP", Currency: "JPY", Exchange: "INDEX"},
		{Symbol: "^HSI", Name: "Hang Seng Index", Country: "HK", Currency: "HKD", Exchange: "INDEX"},
		{Symbol: "^GDAXI", Name: "DAX", Country: "DE", Currency: "EUR", Exchange: "INDEX"},
		{Symbol: "^FCHI", Name: "CAC 40", Country: "FR", Currency: "EUR", Exchange: "INDEX"},
	}

	// Fetch current prices for each index.
	for i := range indices {
		quote, err := c.GetQuote(indices[i].Symbol)
		if err == nil {
			indices[i].CurrentPrice = quote.Current
			indices[i].Change = quote.Change
			indices[i].ChangePercent = quote.ChangePercent
		}
	}

	return indices, nil
}

// GetStocksByExchange returns a list of stocks for a given exchange.
func (c *Client) GetStocksByExchange(exchange string) ([]Stock, error) {
	url := fmt.Sprintf("%s/stock/symbol?exchange=%s&token=%s", c.baseURL, exchange, c.apiKey)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var stocks []Stock
	if err := json.NewDecoder(resp.Body).Decode(&stocks); err != nil {
		return nil, err
	}

	// Limit to first 50 stocks for performance.
	if len(stocks) > 50 {
		stocks = stocks[:50]
	}

	// Fetch current prices for each stock (batch this in production).
	for i := range stocks {
		if i < 10 { // Limit API calls for demo.
			quote, err := c.GetQuote(stocks[i].Symbol)
			if err == nil {
				stocks[i].Price = quote.Current
				stocks[i].Change = quote.Change
				stocks[i].ChangePercent = quote.ChangePercent
			}
		}
	}

	return stocks, nil
}

// GetQuote gets the current quote for a stock.
func (c *Client) GetQuote(symbol string) (*Quote, error) {
	url := fmt.Sprintf("%s/quote?symbol=%s&token=%s", c.baseURL, symbol, c.apiKey)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var quote Quote
	if err := json.NewDecoder(resp.Body).Decode(&quote); err != nil {
		return nil, err
	}

	return &quote, nil
}

// GetHistoricalData returns historical data for a specific stock.
func (c *Client) GetHistoricalData(symbol string, resolution string, from, to int64) (*HistoricalData, error) {
	url := fmt.Sprintf("%s/stock/candle?symbol=%s&resolution=%s&from=%d&to=%d&token=%s",
		c.baseURL, symbol, resolution, from, to, c.apiKey)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var candle Candle
	if err := json.NewDecoder(resp.Body).Decode(&candle); err != nil {
		return nil, err
	}

	if candle.Status != "ok" || len(candle.Timestamp) == 0 {
		return nil, fmt.Errorf("no data available for symbol %s", symbol)
	}

	// Convert to HistoricalData format.
	historical := &HistoricalData{
		Symbol: symbol,
		Data:   make([]DataPoint, len(candle.Timestamp)),
	}

	for i := range candle.Timestamp {
		historical.Data[i] = DataPoint{
			Timestamp: candle.Timestamp[i],
			Date:      time.Unix(candle.Timestamp[i], 0).Format("2006-01-02"),
			Open:      candle.Open[i],
			High:      candle.High[i],
			Low:       candle.Low[i],
			Close:     candle.Close[i],
			Volume:    candle.Volume[i],
		}
	}

	return historical, nil
}

// GetWebSocketURL returns the WebSocket URL for real-time data.
func (c *Client) GetWebSocketURL() string {
	return fmt.Sprintf("wss://ws.finnhub.io?token=%s", c.apiKey)
}