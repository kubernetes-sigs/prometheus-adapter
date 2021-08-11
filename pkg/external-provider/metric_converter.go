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
	"errors"
	"fmt"

	"github.com/prometheus/common/model"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
)

// MetricConverter provides a unified interface for converting the results of
// Prometheus queries into external metric types.
type MetricConverter interface {
	Convert(info provider.ExternalMetricInfo, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error)
}

type metricConverter struct {
}

// NewMetricConverter creates a MetricCoverter, capable of converting any of the three metric types
// returned by the Prometheus client into external metrics types.
func NewMetricConverter() MetricConverter {
	return &metricConverter{}
}

func (c *metricConverter) Convert(info provider.ExternalMetricInfo, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type == model.ValScalar {
		return c.convertScalar(info, queryResult)
	}

	if queryResult.Type == model.ValVector {
		return c.convertVector(info, queryResult)
	}

	return nil, errors.New("encountered an unexpected query result type")
}

func (c *metricConverter) convertSample(info provider.ExternalMetricInfo, sample *model.Sample) (*external_metrics.ExternalMetricValue, error) {
	labels := c.convertLabels(sample.Metric)

	singleMetric := external_metrics.ExternalMetricValue{
		MetricName: info.Metric,
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

func (c *metricConverter) convertVector(info provider.ExternalMetricInfo, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
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
		singleMetric, err := c.convertSample(info, val)

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

func (c *metricConverter) convertScalar(info provider.ExternalMetricInfo, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
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
				MetricName: info.Metric,
				Timestamp: metav1.Time{
					toConvert.Timestamp.Time(),
				},
				Value: *resource.NewMilliQuantity(int64(toConvert.Value*1000.0), resource.DecimalSI),
			},
		},
	}
	return &result, nil
}
