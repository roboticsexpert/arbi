package orderbook

// TradingPair represents a trading pair with base and quote currencies
type TradingPair struct {
	Base  string
	Quote string
}

// String returns the string representation of the trading pair (e.g., "BTC-USDT")
func (p TradingPair) String() string {
	return p.Base + "-" + p.Quote
}

// PriceSource defines the interface that all exchange implementations must satisfy
// Sources automatically connect and stay up, streaming orderbook data continuously
type PriceSource interface {
	// Name returns the price source name
	Name() PriceSourceName

	// GetOrderBook returns the latest orderbook for a trading pair
	GetOrderBook(base, quote string) *OrderBook

	// GetAllOrderBooks returns all orderbooks for this source (key is "base-quote")
	GetAllOrderBooks() map[string]*OrderBook

	// OnUpdate registers a callback for orderbook updates
	OnUpdate(callback func(*OrderBook))
}

// SourceConfig holds configuration for price sources
type SourceConfig struct {
	// Pairs to stream orderbook data for
	Pairs []TradingPair

	// ProxyURL for connections (optional)
	ProxyURL string
}

type PriceSourceName string

const (
	PriceSourceBinance PriceSourceName = "binance"
	PriceSourceKucoin  PriceSourceName = "kucoin"
	PriceSourceEcoGold PriceSourceName = "ecogold"
	PriceSourceNobitex PriceSourceName = "nobitex"
	PriceSourceMT5     PriceSourceName = "mt5"
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
