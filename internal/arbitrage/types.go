package arbitrage

import "arbi/internal/orderbook"

// ConversionRate represents a fixed conversion rate between two assets
type ConversionRate struct {
	From string
	To   string
	Rate float64 // 1 From = Rate To
}

// TradeStep represents a single step in an arbitrage chain
type TradeStep struct {
	Exchange  orderbook.PriceSourceName `json:"exchange"`
	Action    string                    `json:"action"` // "buy" or "sell"
	Base      string                    `json:"base"`
	Quote     string                    `json:"quote"`
	Price     float64                   `json:"price"`      // effective price after orderbook depth
	Amount    float64                   `json:"amount"`     // amount of base currency
	Volume    float64                   `json:"volume"`     // volume in quote currency
	AmountOut float64                   `json:"amount_out"` // what we get after the trade
}

// FixedConversionStep represents a fixed rate conversion (e.g., PAXG -> GOLD18)
type FixedConversionStep struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	Rate      float64 `json:"rate"`
	AmountIn  float64 `json:"amount_in"`
	AmountOut float64 `json:"amount_out"`
}

// ChainStep can be either a trade or a fixed conversion
type ChainStep struct {
	Type            string               `json:"type"` // "trade" or "conversion"
	Trade           *TradeStep           `json:"trade,omitempty"`
	FixedConversion *FixedConversionStep `json:"fixed_conversion,omitempty"`
}

// ArbitrageChain represents a complete arbitrage opportunity
type ArbitrageChain struct {
	Steps         []ChainStep `json:"steps"`
	StartCurrency string      `json:"start_currency"`
	EndCurrency   string      `json:"end_currency"`
	StartAmount   float64     `json:"start_amount"`
	EndAmount     float64     `json:"end_amount"`
	ProfitLoss    float64     `json:"profit_loss"`
	ProfitPercent float64     `json:"profit_percent"`
	Path          string      `json:"path"` // Human readable path like "IRT -> USDT (nobitex) -> PAXG (binance) -> GOLD18 (convert) -> IRT (ecogold)"
}
