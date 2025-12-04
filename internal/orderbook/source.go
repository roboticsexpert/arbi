package orderbook

import "context"

// PriceSource defines the interface that all exchange implementations must satisfy
type PriceSource interface {
	// Name returns the exchange name (e.g., "kucoin", "binance")
	Name() string

	// Subscribe starts receiving orderbook updates for the given symbols
	// Updates are sent to the provided callback
	Subscribe(ctx context.Context, symbols []string, callback func(*OrderBook)) error

	// Unsubscribe stops receiving updates for the given symbol
	Unsubscribe(symbol string) error

	// GetOrderBook returns the latest orderbook for a symbol (if cached)
	GetOrderBook(symbol string) *OrderBook

	// GetAllOrderBooks returns all cached orderbooks for this source
	GetAllOrderBooks() map[string]*OrderBook

	// Start initializes the connection (e.g., WebSocket)
	Start() error

	// Stop closes all connections and cleans up resources
	Stop() error

	// IsRunning returns true if the source is actively connected
	IsRunning() bool
}

// SourceConfig holds common configuration for price sources
type SourceConfig struct {
	// Symbols to subscribe to on startup
	Symbols []string

	// Level of orderbook depth (e.g., 5, 50, full)
	Depth int

	// ProxyURL for connections (optional)
	ProxyURL string
}

