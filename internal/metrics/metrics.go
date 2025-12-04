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

	// BestPrice tracks the best price per exchange, symbol, and side (bid/ask)
	BestPrice = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_best_price",
		Help: "Best price for each exchange, symbol, and side (bid/ask)",
	}, []string{"exchange", "symbol", "side"})

	// Spread tracks the spread (ask - bid) per exchange and symbol
	Spread = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_spread",
		Help: "Spread (best ask - best bid) for each exchange and symbol",
	}, []string{"exchange", "symbol"})

	// OrderbookUpdateTime tracks the last update timestamp per exchange and symbol
	OrderbookUpdateTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "arbi_orderbook_update_timestamp",
		Help: "Last orderbook update timestamp (unix milliseconds) for each exchange and symbol",
	}, []string{"exchange", "symbol"})
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
func UpdateOrderbookMetrics(exchange, symbol string, bidPrice, askPrice float64, timestamp int64) {
	BestPrice.WithLabelValues(exchange, symbol, "bid").Set(bidPrice)
	BestPrice.WithLabelValues(exchange, symbol, "ask").Set(askPrice)
	Spread.WithLabelValues(exchange, symbol).Set(askPrice - bidPrice)
	OrderbookUpdateTime.WithLabelValues(exchange, symbol).Set(float64(timestamp))
}
