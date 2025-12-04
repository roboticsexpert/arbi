package orderbook

// PriceSource defines the interface that all exchange implementations must satisfy
// Sources automatically connect and stay up, streaming orderbook data continuously
type PriceSource interface {
	// Name returns the price source name
	Name() PriceSourceName

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

type PriceSourceName string

const (
	PriceSourceBinance PriceSourceName = "binance"
	PriceSourceKucoin  PriceSourceName = "kucoin"
	PriceSourceEcoGold PriceSourceName = "ecogold"
	PriceSourceNobitex PriceSourceName = "nobitex"
)

// String returns the string value of the price source
func (p PriceSourceName) String() string {
	return string(p)
}

// IsValid checks if the price source is a known/valid source
func (p PriceSourceName) IsValid() bool {
	switch p {
	case PriceSourceBinance, PriceSourceKucoin:
		return true
	default:
		return false
	}
}

// AllPriceSources returns all available price sources
func AllPriceSources() []PriceSourceName {
	return []PriceSourceName{
		PriceSourceBinance,
		PriceSourceKucoin,
	}
}
