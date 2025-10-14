package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// HTTP request metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tapedeck_http_requests_total",
			Help: "Total number of HTTP requests by endpoint and status code",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tapedeck_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	httpErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tapedeck_http_errors_total",
			Help: "Total number of HTTP errors (status >= 400)",
		},
		[]string{"method", "path", "status"},
	)

	// WebSocket metrics
	activeWebSocketConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "tapedeck_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)
)

func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(httpErrorsTotal)
	prometheus.MustRegister(activeWebSocketConnections)
}

// MetricsMiddleware records HTTP request metrics
func MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				written:        false,
			}

			// Call next handler
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)

			httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)

			// Track errors (4xx and 5xx)
			if wrapped.statusCode >= 400 {
				httpErrorsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
			}
		})
	}
}

// IncrementWebSocketConnections increases the active WebSocket connection count
func IncrementWebSocketConnections() {
	activeWebSocketConnections.Inc()
}

// DecrementWebSocketConnections decreases the active WebSocket connection count
func DecrementWebSocketConnections() {
	activeWebSocketConnections.Dec()
}
