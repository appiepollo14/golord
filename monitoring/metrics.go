package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcileDurationTimer = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "golord_func_duration_seconds",
			Help: "Function duration for golords method.",
		},
	)
)

// RegisterMetrics will register metrics with the global prometheus registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(ReconcileDurationTimer)
}
