package provider

import (
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"k8s.io/apimachinery/pkg/labels"
)

//ExportedMetric is a description of an available metric.
type ExportedMetric struct {
	MetricName string
	Labels     labels.Set
	Namespace  string
}

//ExternalInfoMap is a data object that accepts and organizes information
//about available metrics.
type ExternalInfoMap interface {
	//Begins tracking a metric, returning it to the caller.
	TrackMetric(metricName string, generatedBy MetricNamer) ExternalMetricData
	//Exports a collection of all of the metrics currently being tracked.
	ExportMetrics() []ExportedMetric
	//Finds a tracked metric with the given metric name, if it exists.
	FindMetric(metricName string) (data ExternalMetricData, found bool)
}

//ExternalMetricData is a data object that accepts and organizes information
//about the various series/namespaces that a metric is associated with.
type ExternalMetricData interface {
	//MetricName returns the name of the metric represented by this object.
	MetricName() string
	//WithSeries associates the provided labels with this metric.
	WithSeries(labels labels.Set)
	//WithNamespacedSeries associates the provided labels with this metric, but within a particular namespace.
	WithNamespacedSeries(namespace string, labels labels.Set)
	//Exports a collection of all the metrics currently being tracked.
	ExportMetrics() []ExportedMetric
	//Generates a query to select the series/values for the metric this object represents.
	GenerateQuery(namespace string, selector labels.Selector) (prom.Selector, error)
}

type externalInfoMap struct {
	metrics map[string]ExternalMetricData
}

type externalMetricData struct {
	metricName     string
	namespacedData map[string][]labels.Set
	generatedBy    MetricNamer
}

//NewExternalMetricData creates an ExternalMetricData for the provided metric name and namer.
func NewExternalMetricData(metricName string, generatedBy MetricNamer) ExternalMetricData {
	return &externalMetricData{
		metricName:     metricName,
		generatedBy:    generatedBy,
		namespacedData: map[string][]labels.Set{},
	}
}

//NewExternalInfoMap creates an empty ExternalInfoMap for storing external metric information.
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

func (d *externalMetricData) GenerateQuery(namespace string, selector labels.Selector) (prom.Selector, error) {
	return d.generatedBy.QueryForExternalSeries(namespace, d.metricName, selector)
}

func (d *externalMetricData) ExportMetrics() []ExportedMetric {
	results := make([]ExportedMetric, 0)
	for namespace, labelSets := range d.namespacedData {
		for _, labelSet := range labelSets {
			results = append(results, ExportedMetric{
				Labels:     labelSet,
				MetricName: d.metricName,
				Namespace:  namespace,
			})
		}
	}

	return results
}

func (d *externalMetricData) WithSeries(labels labels.Set) {
	d.WithNamespacedSeries("", labels)
}

func (d *externalMetricData) WithNamespacedSeries(namespace string, seriesLabels labels.Set) {
	data, found := d.namespacedData[namespace]
	if !found {
		data = []labels.Set{}
	}

	data = append(data, seriesLabels)
	d.namespacedData[namespace] = data

}
