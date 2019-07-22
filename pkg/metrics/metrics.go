package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
)

const MetricsNamespace = "adapter"

type ServiceMetrics struct {
	PrometheusUp    prometheus.Gauge
	RegistryMetrics *prometheus.GaugeVec
	Errors          *prometheus.CounterVec
	Rules           *prometheus.GaugeVec
	Registry        *prometheus.Registry
}

func NewMetrics() (*ServiceMetrics, error) {
	ret := &ServiceMetrics{
		PrometheusUp: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "prometheus_up",
			Help:      "1 when adapter is able to reach prometheus, 0 otherwise",
		}),

		RegistryMetrics: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "registry_metrics",
			Help:      "number of metrics entries in cache registry",
		}, []string{"registry"}),

		Errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Name:      "errors_total",
			Help:      "number of errors served",
		}, []string{"type"}),

		Rules: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "roles",
			Help:      "number of configured rules",
		}, []string{"type"}),

		Registry: prometheus.NewRegistry(),
	}

	for collectorName, collector := range map[string]prometheus.Collector{
		"Go collector":     prometheus.NewGoCollector(),
		"Prometheus Up":    ret.PrometheusUp,
		"Registry Metrics": ret.RegistryMetrics,
		"Errors":           ret.Errors,
		"Rules":            ret.Rules,
	} {
		if err := ret.Registry.Register(collector); err != nil {
			return nil, fmt.Errorf("during registration of %q: %v", collectorName, err)
		}
	}

	return ret, nil
}

func (m *ServiceMetrics) Run(port uint16) {
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		klog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), mux))
	}()
}
