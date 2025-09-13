# Stock Dashboard Backend

## Description
Go backend service for the real-time stock dashboard application. This service provides REST APIs and WebSocket connections for stock market data.

## Prerequisites
- Go 1.21 or higher
- Finnhub API key (get free at https://finnhub.io/)

## Setup

### 1. Install Dependencies
```bash
go mod download
```

### 2. Environment Variables
Create a `.env` file in the backend directory:
```env
FINNHUB_API_KEY=your_api_key_here
PORT=8080
```

### 3. Run the Server
```bash
# Development
go run main.go

# Build and run
go build -o stock-dashboard
./stock-dashboard
```

## API Endpoints

- `GET /api/health` - Health check endpoint
- `GET /api/v1/stocks/indices` - Get popular stock indices
- `GET /api/v1/stocks/exchange/{symbol}` - Get stocks for an exchange
- `GET /api/v1/stocks/history/{symbol}` - Get historical data for a stock
- `WS /ws` - WebSocket endpoint for real-time stock prices

## Development

### Testing
```bash
go test ./...
```

### Building for Production
```bash
go build -ldflags="-s -w" -o stock-dashboard
```

## Architecture
- `main.go` - Application entry point and server setup
- `handler.go` - HTTP request handlers
- `finnhub/` - Finnhub API client
- `websocket/` - WebSocket server implementation