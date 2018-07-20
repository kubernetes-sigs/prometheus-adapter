package provider

import (
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"k8s.io/apimachinery/pkg/labels"
)

type ExportedMetric struct {
	MetricName string
	Labels     labels.Set
	Namespace  string
}

type ExternalInfoMap interface {
	TrackMetric(metricName string, generatedBy MetricNamer) ExternalMetricData
	ExportMetrics() []ExportedMetric
	FindMetric(metricName string) (data ExternalMetricData, found bool)
}

type ExternalMetricData interface {
	MetricName() string
	WithSeries(labels labels.Set)
	WithNamespacedSeries(namespace string, labels labels.Set)
	ExportMetrics() []ExportedMetric
	GenerateQuery(selector labels.Selector) (prom.Selector, error)
}

type externalInfoMap struct {
	metrics map[string]ExternalMetricData
}

type externalMetricData struct {
	metricName     string
	namespacedData map[string]labels.Set
	generatedBy    MetricNamer
}

func NewExternalMetricData(metricName string, generatedBy MetricNamer) ExternalMetricData {
	return &externalMetricData{
		metricName:     metricName,
		generatedBy:    generatedBy,
		namespacedData: map[string]labels.Set{},
	}
}

func NewExternalInfoMap() ExternalInfoMap {
	return &externalInfoMap{
		metrics: map[string]ExternalMetricData{},
	}
}

func (i *externalInfoMap) ExportMetrics() []ExportedMetric {
	results := make([]ExportedMetric, 0)
	for _, info := range i.metrics {
		exported := info.ExportMetrics()
		results = append(results, exported...)
	}

	return results
}

func (i *externalInfoMap) FindMetric(metricName string) (data ExternalMetricData, found bool) {
	data, found = i.metrics[metricName]
	return data, found
}

func (i *externalInfoMap) TrackMetric(metricName string, generatedBy MetricNamer) ExternalMetricData {
	data, found := i.metrics[metricName]
	if !found {
		data = NewExternalMetricData(metricName, generatedBy)
		i.metrics[metricName] = data
	}

	return data
}

func (d *externalMetricData) MetricName() string {
	return d.metricName
}

func (d *externalMetricData) GenerateQuery(selector labels.Selector) (prom.Selector, error) {
	return d.generatedBy.QueryForExternalSeries(d.metricName, selector)
}

func (d *externalMetricData) ExportMetrics() []ExportedMetric {
	results := make([]ExportedMetric, 0)
	for namespace, labels := range d.namespacedData {
		results = append(results, ExportedMetric{
			Labels:     labels,
			MetricName: d.metricName,
			Namespace:  namespace,
		})
	}

	return results
}

func (d *externalMetricData) WithSeries(labels labels.Set) {
	d.WithNamespacedSeries("", labels)
}

func (d *externalMetricData) WithNamespacedSeries(namespace string, labels labels.Set) {
	data, found := d.namespacedData[namespace]
	if !found {
		data = labels
		d.namespacedData[namespace] = data
	}
}
