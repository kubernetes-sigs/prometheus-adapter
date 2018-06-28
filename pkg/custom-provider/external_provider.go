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
	"fmt"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/external_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

type externalPrometheusProvider struct {
	mapper       apimeta.RESTMapper
	kubeClient   dynamic.Interface
	promClient   prom.Client
	queryBuilder ExternalMetricQueryBuilder

	SeriesRegistry
}

func NewExternalPrometheusProvider(mapper apimeta.RESTMapper, kubeClient dynamic.Interface, promClient prom.Client, namers []MetricNamer, updateInterval time.Duration, queryBuilder ExternalMetricQueryBuilder) (provider.ExternalMetricsProvider, Runnable) {
	lister := &cachingMetricsLister{
		updateInterval: updateInterval,
		promClient:     promClient,
		namers:         namers,

		SeriesRegistry: &basicSeriesRegistry{
			mapper: mapper,
		},
	}

	return &externalPrometheusProvider{
		mapper:     mapper,
		kubeClient: kubeClient,
		promClient: promClient,

		SeriesRegistry: lister,
	}, lister
}

func (p *externalPrometheusProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	//TODO: Steps
	//1. Generate a Prometheus Query.
	//   Something like my_metric{namespace="namespace" some_label="some_value"}
	//2. Send that query to Prometheus.
	//3. Adapt the results.
	//The query generation for external metrics is much more straightforward
	//than for custom metrics because no renaming is applied.
	//So we'll just start with some simple string operations and see how far that gets us.
	//Then I'll circle back and figure out how much code reuse I can get out of the original implementation.
	query := p.queryBuilder.BuildPrometheusQuery(namespace, metricName, metricSelector)

	//TODO: Only here to stop compiler issues in this incomplete code.
	fmt.Printf(query)

	//TODO: Construct a real result.
	return nil, nil
}

func (p *externalPrometheusProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	//TODO: Provide a real response.
	return nil
}
