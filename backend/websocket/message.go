package websocket

// Message represents a generic WebSocket message structure.
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// SubscribeMessage represents a subscription request from a client.
type SubscribeMessage struct {
	Type    string   `json:"type"`
	Symbols []string `json:"symbols"`
}

// TradeData represents real-time trade data from Finnhub.
type TradeData struct {
	Symbol    string  `json:"s"`
	Price     float64 `json:"p"`
	Timestamp int64   `json:"t"`
	Volume    int64   `json:"v"`
}

// QuoteData represents real-time quote data.
// Note: This struct is defined but not used in the message processing logic.
// It can be used if you decide to handle quote data streams in the future.
type QuoteData struct {
	Symbol        string  `json:"s"`
	CurrentPrice  float64 `json:"c"`
	Change        float64 `json:"d"`
	ChangePercent float64 `json:"dp"`
	High          float64 `json:"h"`
	Low           float64 `json:"l"`
	Open          float64 `json:"o"`
	PreviousClose float64 `json:"pc"`
	Timestamp     int64   `json:"t"`
}