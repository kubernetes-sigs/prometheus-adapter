/*
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"sync"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"
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
	// We lock when reading/writing metrics, and metricsInfo to prevent inconsistencies.
	mu sync.RWMutex

	// metrics is the list of all known metrics, ready to return from the API
	metrics []provider.ExternalMetricInfo
	// metricsInfo is a lookup from a metric to SeriesConverter for the sake of generating queries
	metricsInfo map[string]seriesInfo
}

type seriesInfo struct {
	// seriesName is the name of the corresponding Prometheus series
	seriesName string

	// namer is the MetricNamer used to name this series
	namer naming.MetricNamer
}

// NewExternalSeriesRegistry creates an ExternalSeriesRegistry driven by the data from the provided MetricLister.
func NewExternalSeriesRegistry(lister MetricListerWithNotification) ExternalSeriesRegistry {
	var registry = externalSeriesRegistry{
		metrics:     make([]provider.ExternalMetricInfo, 0),
		metricsInfo: map[string]seriesInfo{},
	}

	lister.AddNotificationReceiver(registry.filterAndStoreMetrics)

	return &registry
}

func (r *externalSeriesRegistry) filterAndStoreMetrics(result MetricUpdateResult) {
	newSeriesSlices := result.series
	namers := result.namers

	if len(newSeriesSlices) != len(namers) {
		klog.Fatal("need one set of series per converter")
	}
	apiMetricsCache := make([]provider.ExternalMetricInfo, 0)
	rawMetricsCache := make(map[string]seriesInfo)

	for i, newSeries := range newSeriesSlices {
		namer := namers[i]
		for _, series := range newSeries {
			identity, err := namer.MetricNameForSeries(series)

			if err != nil {
				klog.Errorf("unable to name series %q, skipping: %v", series.String(), err)
				continue
			}

			name := identity
			rawMetricsCache[name] = seriesInfo{
				seriesName: series.Name,
				namer:      namer,
			}
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
	r.metricsInfo = rawMetricsCache

}

func (r *externalSeriesRegistry) ListAllMetrics() []provider.ExternalMetricInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.metrics
}

func (r *externalSeriesRegistry) QueryForMetric(namespace string, metricName string, metricSelector labels.Selector) (prom.Selector, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, found := r.metricsInfo[metricName]

	if !found {
		klog.V(10).Infof("external metric %q not found", metricName)
		return "", false, nil
	}
	query, err := info.namer.QueryForExternalSeries(info.seriesName, namespace, metricSelector)

	return query, found, err
}
