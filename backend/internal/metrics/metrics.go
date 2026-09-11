package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	startTime = time.Now()

	// UptimeSeconds tracks application uptime in seconds
	UptimeSeconds = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "arbi_uptime_seconds",
		Help: "Application uptime in seconds",
	}, func() float64 {
		return time.Since(startTime).Seconds()
	})

	// RequestsTotal counts total HTTP requests (example for future use)
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "arbi_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "endpoint", "status"})

	// ActiveConnections tracks current active connections (example gauge)
	ActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "arbi_active_connections",
		Help: "Number of active connections",
	})

	// BestPrice tracks the best price per exchange, base, quote, and side (bid/ask)
	BestPrice = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_best_price",
		Help: "Best price for each exchange, base, quote, and side (bid/ask)",
	}, []string{"exchange", "base", "quote", "side"})

	// Spread tracks the spread (ask - bid) per exchange, base and quote
	Spread = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_spread",
		Help: "Spread (best ask - best bid) for each exchange, base and quote",
	}, []string{"exchange", "base", "quote"})

	// OrderbookUpdateTime tracks the last update timestamp per exchange, base and quote
	OrderbookUpdateTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_orderbook_update_timestamp",
		Help: "Last orderbook update timestamp (unix seconds) for each exchange, base and quote",
	}, []string{"exchange", "base", "quote"})

	// InventoryLastFetchTime tracks the last time we successfully fetched inventory/balance per exchange
	InventoryLastFetchTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_inventory_last_fetch_timestamp",
		Help: "Last inventory/balance fetch timestamp (unix seconds) for each price source",
	}, []string{"exchange"})

	// ArbitrageProfit tracks the profit/loss percentage for each arbitrage chain (with fee)
	ArbitrageProfit = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_arbitrage_profit_percent",
		Help: "Profit/loss percentage for each arbitrage chain path (with fee)",
	}, []string{"path"})

	// ArbitrageProfitIRT tracks the profit/loss in IRT for each arbitrage chain (with fee)
	ArbitrageProfitIRT = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_arbitrage_profit_irt",
		Help: "Profit/loss in IRT for each arbitrage chain path (with fee)",
	}, []string{"path"})

	// ArbitrageProfitNoFee tracks the profit/loss percentage for each arbitrage chain (without fee)
	ArbitrageProfitNoFee = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_arbitrage_profit_percent_no_fee",
		Help: "Profit/loss percentage for each arbitrage chain path (without fee)",
	}, []string{"path"})

	// ArbitrageProfitIRTNoFee tracks the profit/loss in IRT for each arbitrage chain (without fee)
	ArbitrageProfitIRTNoFee = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_arbitrage_profit_irt_no_fee",
		Help: "Profit/loss in IRT for each arbitrage chain path (without fee)",
	}, []string{"path"})

	// WalletBalance tracks wallet balance per exchange and currency
	WalletBalance = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_wallet_balance",
		Help: "Wallet balance for each exchange and currency",
	}, []string{"exchange", "currency"})
)

// IncrementRequests increments the request counter
func IncrementRequests(method, endpoint, status string) {
	RequestsTotal.WithLabelValues(method, endpoint, status).Inc()
}

// SetActiveConnections sets the active connections gauge
func SetActiveConnections(count float64) {
	ActiveConnections.Set(count)
}

// UpdateOrderbookMetrics updates all orderbook-related metrics
func UpdateOrderbookMetrics(exchange, base, quote string, bidPrice, askPrice float64, timestamp int64) {
	BestPrice.WithLabelValues(exchange, base, quote, "bid").Set(bidPrice)
	BestPrice.WithLabelValues(exchange, base, quote, "ask").Set(askPrice)
	Spread.WithLabelValues(exchange, base, quote).Set(askPrice - bidPrice)
	OrderbookUpdateTime.WithLabelValues(exchange, base, quote).Set(float64(timestamp))
}

// UpdateArbitrageMetrics updates the arbitrage chain metrics (with fee)
func UpdateArbitrageMetrics(path string, profitPercent, profitIRT float64) {
	ArbitrageProfit.WithLabelValues(path).Set(profitPercent)
	ArbitrageProfitIRT.WithLabelValues(path).Set(profitIRT)
}

// UpdateArbitrageMetricsNoFee updates the arbitrage chain metrics (without fee)
func UpdateArbitrageMetricsNoFee(path string, profitPercent, profitIRT float64) {
	ArbitrageProfitNoFee.WithLabelValues(path).Set(profitPercent)
	ArbitrageProfitIRTNoFee.WithLabelValues(path).Set(profitIRT)
}

// ResetArbitrageMetrics resets all arbitrage metrics (call before updating with new chains)
func ResetArbitrageMetrics() {
	ArbitrageProfit.Reset()
	ArbitrageProfitIRT.Reset()
	ArbitrageProfitNoFee.Reset()
	ArbitrageProfitIRTNoFee.Reset()
}

// UpdateWalletBalanceMetrics updates wallet balance metrics for an exchange
func UpdateWalletBalanceMetrics(exchange string, balances map[string]float64) {
	storeBalanceSnapshot(exchange, balances)
	for currency, balance := range balances {
		WalletBalance.WithLabelValues(exchange, currency).Set(balance)
	}
	InventoryLastFetchTime.WithLabelValues(exchange).Set(float64(time.Now().Unix()))
}
