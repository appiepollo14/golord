package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	MemcachedDeploymentSizeUndesiredCountTotal = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "memcached_deployment_size_undesired_count_total",
			Help: "Total number of times the deployment size was not as desired.",
		},
	)
)

// RegisterMetrics will register metrics with the global prometheus registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(MemcachedDeploymentSizeUndesiredCountTotal)
}