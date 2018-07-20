package provider

import (
	"sync"

	"github.com/prometheus/common/model"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

//ExternalSeriesRegistry acts as the top-level converter for transforming Kubernetes requests
//for external metrics into Prometheus queries.
type ExternalSeriesRegistry interface {
	// ListAllMetrics lists all metrics known to this registry
	ListAllMetrics() []provider.ExternalMetricInfo
	QueryForMetric(namespace string, metricName string, metricSelector labels.Selector) (query prom.Selector, found bool)
}

// overridableSeriesRegistry is a basic SeriesRegistry
type externalSeriesRegistry struct {
	mu sync.RWMutex

	// metrics is the list of all known metrics
	metrics []provider.ExternalMetricInfo

	mapper apimeta.RESTMapper

	metricLister       MetricListerWithNotification
	externalMetricInfo ExternalInfoMap
}

//NewExternalSeriesRegistry creates an ExternalSeriesRegistry driven by the data from the provided MetricLister.
func NewExternalSeriesRegistry(lister MetricListerWithNotification, mapper apimeta.RESTMapper) ExternalSeriesRegistry {
	var registry = externalSeriesRegistry{
		mapper:             mapper,
		metricLister:       lister,
		externalMetricInfo: NewExternalInfoMap(),
	}

	lister.AddNotificationReceiver(registry.onNewDataAvailable)

	return &registry
}

func (r *externalSeriesRegistry) filterMetrics(result metricUpdateResult) metricUpdateResult {
	namers := make([]MetricNamer, 0)
	series := make([][]prom.Series, 0)

	targetType := config.External

	for i, namer := range result.namers {
		if namer.MetricType() == targetType {
			namers = append(namers, namer)
			series = append(series, result.series[i])
		}
	}

	return metricUpdateResult{
		namers: namers,
		series: series,
	}
}

func (r *externalSeriesRegistry) convertLabels(labels model.LabelSet) labels.Set {
	set := map[string]string{}
	for key, value := range labels {
		set[string(key)] = string(value)
	}
	return set
}

func (r *externalSeriesRegistry) onNewDataAvailable(result metricUpdateResult) {
	result = r.filterMetrics(result)

	newSeriesSlices := result.series
	namers := result.namers

	// if len(newSeriesSlices) != len(namers) {
	// 	return fmt.Errorf("need one set of series per namer")
	// }

	updatedCache := NewExternalInfoMap()
	for i, newSeries := range newSeriesSlices {
		namer := namers[i]
		for _, series := range newSeries {
			identity, err := namer.IdentifySeries(series)

			if err != nil {
				glog.Errorf("unable to name series %q, skipping: %v", series.String(), err)
				continue
			}

			// resources := identity.resources
			// namespaced := identity.namespaced
			name := identity.name
			labels := r.convertLabels(series.Labels)

			//Check for a label indicating namespace.
			metricNs, found := series.Labels[model.LabelName(namer.ExternalMetricNamespaceLabelName())]

			if !found {
				metricNs = ""
			}

			trackedMetric := updatedCache.TrackMetric(name, namer)
			trackedMetric.WithNamespacedSeries(string(metricNs), labels)
		}
	}

	// regenerate metrics
	allMetrics := updatedCache.ExportMetrics()
	convertedMetrics := r.convertMetrics(allMetrics)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.externalMetricInfo = updatedCache
	r.metrics = convertedMetrics
}

func (r *externalSeriesRegistry) convertMetrics(metrics []ExportedMetric) []provider.ExternalMetricInfo {
	results := make([]provider.ExternalMetricInfo, len(metrics))
	for i, info := range metrics {
		results[i] = provider.ExternalMetricInfo{
			Labels: info.Labels,
			Metric: info.MetricName,
		}
	}

	return results
}

func (r *externalSeriesRegistry) ListAllMetrics() []provider.ExternalMetricInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.metrics
}

func (r *externalSeriesRegistry) QueryForMetric(namespace string, metricName string, metricSelector labels.Selector) (query prom.Selector, found bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metric, found := r.externalMetricInfo.FindMetric(metricName)

	if !found {
		glog.V(10).Infof("external metric %q not registered", metricName)
		return "", false
	}

	query, err := metric.GenerateQuery(namespace, metricSelector)

	if err != nil {
		glog.Errorf("unable to construct query for external metric %s: %v", metricName, err)
		return "", false
	}

	return query, true
}
