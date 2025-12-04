package orderbook

import "time"

// PriceLevel represents a single price level in the order book
type PriceLevel struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

// OrderBook represents the current state of an order book
type OrderBook struct {
	Source    PriceSourceName `json:"source"`
	Symbol    string          `json:"symbol"`
	Base      string          `json:"base"`
	Quote     string          `json:"quote"`
	Bids      []PriceLevel    `json:"bids"`
	Asks      []PriceLevel    `json:"asks"`
	Timestamp int64           `json:"timestamp"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// OrderBookKey uniquely identifies an orderbook
type OrderBookKey struct {
	Source PriceSourceName
	Symbol string
}

// String returns a string representation of the key
func (k OrderBookKey) String() string {
	return k.Source.String() + ":" + k.Symbol
}

// BestBid returns the best (highest) bid price level, or nil if no bids
func (ob *OrderBook) BestBid() *PriceLevel {
	if len(ob.Bids) == 0 {
		return nil
	}
	return &ob.Bids[0]
}

// BestAsk returns the best (lowest) ask price level, or nil if no asks
func (ob *OrderBook) BestAsk() *PriceLevel {
	if len(ob.Asks) == 0 {
		return nil
	}
	return &ob.Asks[0]
}
