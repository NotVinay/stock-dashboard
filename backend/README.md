# Stock Dashboard Backend

This is the Go backend service for the real-time stock dashboard application. It provides REST APIs for historical stock data and a WebSocket server for real-time price updates, acting as a proxy and manager for the [Finnhub.io](https://finnhub.io/) API.

## Features

- **REST API**: Provides endpoints for fetching stock indices, exchange data, historical prices, and quotes.
- **WebSocket Server**: Manages client connections and streams real-time trade data for subscribed stock symbols.
- **Finnhub Integration**: Efficiently communicates with the Finnhub API, managing a single upstream WebSocket connection to broadcast data to multiple clients.
- **Configuration-driven**: Easily configurable via environment variables or a `.env` file.
- **Graceful Shutdown**: Ensures clean shutdown of server and connections.

## Prerequisites

- Go 1.21 or higher
- A free **Finnhub API key** from finnhub.io.

## Getting Started

### 1. Installation

Clone the repository and install the dependencies:

```bash
go mod download
```

### 2. Environment Variables

Create a `.env` file in the backend directory:

```env
FINNHUB_API_KEY=your_api_key_here
FINNHUB_API_URL=https://finnhub.io/api/v1
ENV=development
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
- `config/` - Loads environment config for the application.
