package orderbook

// PriceSource defines the interface that all exchange implementations must satisfy
// Sources automatically connect and stay up, streaming orderbook data continuously
type PriceSource interface {
	// Name returns the exchange name (e.g., "kucoin", "binance")
	Name() string

	// GetOrderBook returns the latest orderbook for a symbol
	GetOrderBook(symbol string) *OrderBook

	// GetAllOrderBooks returns all orderbooks for this source
	GetAllOrderBooks() map[string]*OrderBook

	// OnUpdate registers a callback for orderbook updates
	OnUpdate(callback func(*OrderBook))
}

// SourceConfig holds configuration for price sources
type SourceConfig struct {
	// Symbols to stream orderbook data for
	Symbols []string

	// ProxyURL for connections (optional)
	ProxyURL string
}
