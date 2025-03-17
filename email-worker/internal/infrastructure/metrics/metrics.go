package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {

	// ObserveHTTPRequestDuration records the duration of an HTTP request.
	ObserveHTTPRequestDuration(topic string, success bool, duration float64)
}

type prometheusMetrics struct {
	consumptionDuration *prometheus.HistogramVec
}

func NewMetrics() Metrics {
	hv := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kafka_consumer_duration_seconds",
			Help:    "Duration of message consumption in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic", "success"},
	)
	prometheus.MustRegister(hv)
	return &prometheusMetrics{consumptionDuration: hv}
}

func (pm *prometheusMetrics) ObserveHTTPRequestDuration(topic string, success bool, duration float64) {
	pm.consumptionDuration.
		With(prometheus.Labels{
			"topic":   topic,
			"success": strconv.FormatBool(success),
		}).
		Observe(duration)
}

// Ensure PrometheusMetrics satisfies the Metrics interface.
var _ Metrics = (*prometheusMetrics)(nil)
