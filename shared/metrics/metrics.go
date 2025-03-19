package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	// ObserveHTTPRequestDuration records the duration of an HTTP request.
	ObserveKafkaConsumerDuration(topic string, success bool, duration float64)

	// ObserveHTTPRequestDuration records the duration of an HTTP request.
	ObserveHTTPRequestDuration(handler, method, status string, duration float64)
}

type prometheusMetrics struct {
	consumptionDuration *prometheus.HistogramVec
	httpDuration        *prometheus.HistogramVec
}

func NewMetrics() Metrics {
	hvConsumer := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kafka_consumer_duration_seconds",
			Help:    "Duration of message consumption in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic", "success"},
	)
	hv := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler", "method", "status"},
	)
	prometheus.MustRegister(hv)
	return &prometheusMetrics{
		consumptionDuration: hvConsumer,
		httpDuration:        hv,
	}
}

func (pm *prometheusMetrics) ObserveKafkaConsumerDuration(topic string, success bool, duration float64) {
	pm.consumptionDuration.
		With(prometheus.Labels{
			"topic":   topic,
			"success": strconv.FormatBool(success),
		}).
		Observe(duration)
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
