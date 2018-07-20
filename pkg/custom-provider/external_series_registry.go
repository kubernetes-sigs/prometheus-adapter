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

type ExternalSeriesRegistry interface {
	// ListAllMetrics lists all metrics known to this registry
	ListAllMetrics() []provider.ExternalMetricInfo
	QueryForMetric(metricName string, metricSelector labels.Selector) (query prom.Selector, found bool)
}

// overridableSeriesRegistry is a basic SeriesRegistry
type externalSeriesRegistry struct {
	mu sync.RWMutex

	externalInfo map[string]seriesInfo
	// metrics is the list of all known metrics
	metrics []provider.ExternalMetricInfo

	mapper apimeta.RESTMapper

	metricLister     MetricListerWithNotification
	tonyExternalInfo ExternalInfoMap
}

func NewExternalSeriesRegistry(lister MetricListerWithNotification, mapper apimeta.RESTMapper) ExternalSeriesRegistry {
	var registry = externalSeriesRegistry{
		mapper:           mapper,
		metricLister:     lister,
		tonyExternalInfo: NewExternalInfoMap(),
	}

	lister.AddNotificationReceiver(registry.onNewDataAvailable)

	return &registry
}

func (r *externalSeriesRegistry) filterMetrics(result metricUpdateResult) metricUpdateResult {
	namers := make([]MetricNamer, 0)
	series := make([][]prom.Series, 0)

	targetType := config.MetricType("External")

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
			//TODO: Figure out the namespace, if applicable
			metricNs := ""
			trackedMetric := updatedCache.TrackMetric(name, namer)
			trackedMetric.WithNamespacedSeries(metricNs, labels)
		}
	}

	// regenerate metrics
	allMetrics := updatedCache.ExportMetrics()
	convertedMetrics := r.convertMetrics(allMetrics)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tonyExternalInfo = updatedCache
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

func (r *externalSeriesRegistry) QueryForMetric(metricName string, metricSelector labels.Selector) (query prom.Selector, found bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metric, found := r.tonyExternalInfo.FindMetric(metricName)

	if !found {
		return "", false
	}

	query, err := metric.GenerateQuery(metricSelector)
	// info, infoFound := r.info[metricInfo]
	// if !infoFound {
	// 	//TODO: Weird that it switches between types here.
	// 	glog.V(10).Infof("metric %v not registered", metricInfo)
	// 	return "", false
	// }

	// query, err := info.namer.QueryForExternalSeries(info.seriesName, metricSelector)
	if err != nil {
		//TODO: See what was being .String() and implement that for ExternalMetricInfo.
		// errorVal := metricInfo.String()
		errorVal := "something"
		glog.Errorf("unable to construct query for metric %s: %v", errorVal, err)
		return "", false
	}

	return query, true
}
