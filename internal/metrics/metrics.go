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
)

// IncrementRequests increments the request counter
func IncrementRequests(method, endpoint, status string) {
	RequestsTotal.WithLabelValues(method, endpoint, status).Inc()
}

// SetActiveConnections sets the active connections gauge
func SetActiveConnections(count float64) {
	ActiveConnections.Set(count)
}
