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
		Help: "Last orderbook update timestamp (unix milliseconds) for each exchange, base and quote",
	}, []string{"exchange", "base", "quote"})
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
