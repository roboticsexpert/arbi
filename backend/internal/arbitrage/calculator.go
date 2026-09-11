package arbitrage

import (
	"math"
	"strconv"

	"arbi/internal/orderbook"
)

// CalculateTradeOutput calculates the output amount when trading through an orderbook
// For buying base with quote: we spend 'amount' of quote currency, get base
// For selling base for quote: we spend 'amount' of base currency, get quote
func CalculateTradeOutput(ob *orderbook.OrderBook, action string, inputAmount float64) (outputAmount float64, effectivePrice float64) {
	if ob == nil {
		return 0, 0
	}

	var levels []orderbook.PriceLevel
	if action == "buy" {
		// We're buying base with quote, use asks (we pay ask price)
		levels = ob.Asks
	} else {
		// We're selling base for quote, use bids (we receive bid price)
		levels = ob.Bids
	}

	if len(levels) == 0 {
		return 0, 0
	}

	remaining := inputAmount
	totalOutput := 0.0
	totalInput := 0.0

	for _, level := range levels {
		price, err := strconv.ParseFloat(level.Price, 64)
		if err != nil || price <= 0 {
			continue
		}
		quantity, err := strconv.ParseFloat(level.Quantity, 64)
		if err != nil || quantity <= 0 {
			continue
		}

		if action == "buy" {
			// We're spending quote (inputAmount is in quote currency)
			// At this price level, max quote we can spend = price * quantity
			availableQuote := price * quantity
			if remaining <= availableQuote {
				// We can fill the remaining here
				baseWeGet := remaining / price
				totalOutput += baseWeGet
				totalInput += remaining
				remaining = 0
				break
			} else {
				// Take all at this level
				totalOutput += quantity
				totalInput += availableQuote
				remaining -= availableQuote
			}
		} else {
			// We're spending base (inputAmount is in base currency)
			// At this price level, max base we can sell = quantity
			if remaining <= quantity {
				// We can fill the remaining here
				quoteWeGet := remaining * price
				totalOutput += quoteWeGet
				totalInput += remaining
				remaining = 0
				break
			} else {
				// Take all at this level
				totalOutput += quantity * price
				totalInput += quantity
				remaining -= quantity
			}
		}
	}

	if totalInput == 0 {
		return 0, 0
	}

	if action == "buy" {
		// Effective price = total quote spent / total base received
		effectivePrice = totalInput / totalOutput
	} else {
		// Effective price = total quote received / total base sold
		effectivePrice = totalOutput / totalInput
	}

	return totalOutput, effectivePrice
}

// CalculateChain calculates the complete arbitrage chain
func CalculateChain(steps []ChainStep, startAmount float64) (endAmount float64, ok bool) {
	current := startAmount

	for _, step := range steps {
		if step.Type == "trade" && step.Trade != nil {
			if step.Trade.AmountOut <= 0 {
				return 0, false
			}
			current = step.Trade.AmountOut
		} else if step.Type == "conversion" && step.FixedConversion != nil {
			if step.FixedConversion.AmountOut <= 0 {
				return 0, false
			}
			current = step.FixedConversion.AmountOut
		} else {
			return 0, false
		}
	}

	return current, true
}

// FormatNumber formats a number with appropriate precision
func FormatNumber(n float64) string {
	if math.Abs(n) >= 1000000 {
		return strconv.FormatFloat(n, 'f', 0, 64)
	} else if math.Abs(n) >= 1 {
		return strconv.FormatFloat(n, 'f', 2, 64)
	} else if math.Abs(n) >= 0.0001 {
		return strconv.FormatFloat(n, 'f', 6, 64)
	}
	return strconv.FormatFloat(n, 'f', 10, 64)
}
