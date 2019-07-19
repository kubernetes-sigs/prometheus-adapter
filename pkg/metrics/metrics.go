package metrics

import "github.com/prometheus/client_golang/prometheus"

const MetricsNamespace = "adapter"

var PrometheusUp = prometheus.NewGauge(prometheus.GaugeOpts{
	Namespace: MetricsNamespace,
	Name:      "prometheus_up",
	Help:      "1 when adapter is able to reach prometheus, 0 otherwise",
})

var RegistryMetrics = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricsNamespace,
	Name:      "registry_metrics",
	Help:      "number of metrics entries in cache registry",
}, []string{"registry"})

var Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: MetricsNamespace,
	Name:      "errors_total",
	Help:      "number of errors served",
}, []string{"type"})

var Rules = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: MetricsNamespace,
	Name:      "roles",
	Help:      "number of configured rules",
}, []string{"type"})

func init() {
	prometheus.MustRegister(
		PrometheusUp,
		RegistryMetrics,
		Errors,
		Rules,
	)
}
