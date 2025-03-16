package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {

	// ObserveHTTPRequestDuration records the duration of an HTTP request.
	ObserveHTTPRequestDuration(handler, method, status string, duration float64)
}

type prometheusMetrics struct {
	httpDuration *prometheus.HistogramVec
}

func NewMetrics() Metrics {
	hv := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler", "method", "status"},
	)
	prometheus.MustRegister(hv)
	return &prometheusMetrics{httpDuration: hv}
}

func (pm *prometheusMetrics) ObserveHTTPRequestDuration(handler, method, status string, duration float64) {
	pm.httpDuration.
		With(prometheus.Labels{
			"handler": handler,
			"method":  method,
			"status":  status,
		}).
		Observe(duration)
}

// Ensure PrometheusMetrics satisfies the Metrics interface.
var _ Metrics = (*prometheusMetrics)(nil)
