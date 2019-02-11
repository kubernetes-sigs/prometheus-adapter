package provider

import (
	"sync"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// ExternalSeriesRegistry acts as the top-level converter for transforming Kubernetes requests
// for external metrics into Prometheus queries.
type ExternalSeriesRegistry interface {
	// ListAllMetrics lists all metrics known to this registry
	ListAllMetrics() []provider.ExternalMetricInfo
	QueryForMetric(namespace string, metricName string, metricSelector labels.Selector) (prom.Selector, bool, error)
}

// overridableSeriesRegistry is a basic SeriesRegistry
type externalSeriesRegistry struct {
	// We lock when reading/writing metrics, and rawMetrics to prevent inconsistencies.
	mu sync.RWMutex

	// metrics is the list of all known metrics, ready to return from the API
	metrics []provider.ExternalMetricInfo
	// rawMetrics is a lookup from a metric to SeriesConverter for the sake of generating queries
	rawMetrics map[string]SeriesConverter

	mapper apimeta.RESTMapper

	metricLister MetricListerWithNotification
}

// NewExternalSeriesRegistry creates an ExternalSeriesRegistry driven by the data from the provided MetricLister.
func NewExternalSeriesRegistry(lister MetricListerWithNotification, mapper apimeta.RESTMapper) ExternalSeriesRegistry {
	var registry = externalSeriesRegistry{
		mapper:       mapper,
		metricLister: lister,
		metrics:      make([]provider.ExternalMetricInfo, 0),
		rawMetrics:   map[string]SeriesConverter{},
	}

	lister.AddNotificationReceiver(registry.filterAndStoreMetrics)

	return &registry
}

func (r *externalSeriesRegistry) filterAndStoreMetrics(result MetricUpdateResult) {
	newSeriesSlices := result.series
	converters := result.converters

	if len(newSeriesSlices) != len(converters) {
		glog.Fatal("need one set of series per converter")
	}
	apiMetricsCache := make([]provider.ExternalMetricInfo, 0)
	rawMetricsCache := make(map[string]SeriesConverter)

	for i, newSeries := range newSeriesSlices {
		converter := converters[i]
		for _, series := range newSeries {
			identity, err := converter.IdentifySeries(series)

			if err != nil {
				glog.Errorf("unable to name series %q, skipping: %v", series.String(), err)
				continue
			}

			name := identity.name
			rawMetricsCache[name] = converter
		}
	}

	for metricName := range rawMetricsCache {
		apiMetricsCache = append(apiMetricsCache, provider.ExternalMetricInfo{
			Metric: metricName,
		})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.metrics = apiMetricsCache
	r.rawMetrics = rawMetricsCache

}

func (r *externalSeriesRegistry) ListAllMetrics() []provider.ExternalMetricInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.metrics
}

func (r *externalSeriesRegistry) QueryForMetric(namespace string, metricName string, metricSelector labels.Selector) (prom.Selector, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	converter, found := r.rawMetrics[metricName]

	if !found {
		glog.V(10).Infof("external metric %q not found", metricName)
		return "", false, nil
	}

	query, err := converter.QueryForExternalSeries(namespace, metricName, metricSelector)
	return query, found, err
}
