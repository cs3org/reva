package blobstore

import "github.com/prometheus/client_golang/prometheus"

var (
	// Namespace defines the namespace for the defines metrics.
	Namespace = "ocis"

	// Subsystem defines the subsystem for the defines metrics.
	Subsystem = "s3ng"
)

// Metrics defines the available metrics of this service.
type Metrics struct {
	Rx *prometheus.CounterVec
	Tx *prometheus.CounterVec
}

// NewMetrics initializes the available metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		Rx: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "Rx",
			Help:      "Storage access rx",
		}, []string{}),
		Tx: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "Tx",
			Help:      "Storage access tx",
		}, []string{}),
	}
	_ = prometheus.Register(m.Rx)
	_ = prometheus.Register(m.Tx)

	return m
}
