package provider

import (
	"errors"
	"fmt"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

// MetricConverter provides a unified interface for converting the results of
// Prometheus queries into external metric types.
type MetricConverter interface {
	Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error)
}

type metricConverter struct {
}

// NewMetricConverter creates a MetricCoverter, capable of converting any of the three metric types
// returned by the Prometheus client into external metrics types.
func NewMetricConverter() MetricConverter {
	return &metricConverter{}
}

func (c *metricConverter) Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type == model.ValScalar {
		return c.convertScalar(queryResult)
	}

	if queryResult.Type == model.ValVector {
		return c.convertVector(queryResult)
	}

	return nil, errors.New("encountered an unexpected query result type")
}

func (c *metricConverter) convertSample(sample *model.Sample) (*external_metrics.ExternalMetricValue, error) {
	labels := c.convertLabels(sample.Metric)

	singleMetric := external_metrics.ExternalMetricValue{
		MetricName: string(sample.Metric[model.LabelName("__name__")]),
		Timestamp: metav1.Time{
			sample.Timestamp.Time(),
		},
		Value:        *resource.NewMilliQuantity(int64(sample.Value*1000.0), resource.DecimalSI),
		MetricLabels: labels,
	}

	return &singleMetric, nil
}

func (c *metricConverter) convertLabels(inLabels model.Metric) map[string]string {
	numLabels := len(inLabels)
	outLabels := make(map[string]string, numLabels)
	for labelName, labelVal := range inLabels {
		outLabels[string(labelName)] = string(labelVal)
	}

	return outLabels
}

func (c *metricConverter) convertVector(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValVector {
		return nil, errors.New("incorrect query result type")
	}

	toConvert := *queryResult.Vector

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain vector query results")
	}

	items := []external_metrics.ExternalMetricValue{}
	metricValueList := external_metrics.ExternalMetricValueList{
		Items: items,
	}

	numSamples := toConvert.Len()
	if numSamples == 0 {
		return &metricValueList, nil
	}

	for _, val := range toConvert {

		singleMetric, err := c.convertSample(val)

		if err != nil {
			return nil, fmt.Errorf("unable to convert vector: %v", err)
		}

		items = append(items, *singleMetric)
	}

	metricValueList = external_metrics.ExternalMetricValueList{
		Items: items,
	}
	return &metricValueList, nil
}

func (c *metricConverter) convertScalar(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValScalar {
		return nil, errors.New("scalarConverter can only convert scalar query results")
	}

	toConvert := queryResult.Scalar

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain scalar query results")
	}

	result := external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{
			{
				Timestamp: metav1.Time{
					toConvert.Timestamp.Time(),
				},
				Value: *resource.NewMilliQuantity(int64(toConvert.Value*1000.0), resource.DecimalSI),
			},
		},
	}
	return &result, nil
}
