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
	"context"
	"fmt"
	"time"

	pmodel "github.com/prometheus/common/model"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/external_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

//TODO: Some of these members may not be necessary.
//Some of them are definitely duplicated between the
//external and custom providers. They should probably share
//the same instances of these objects (especially the SeriesRegistry)
//to cut down on unnecessary chatter/bookkeeping.
type externalPrometheusProvider struct {
	mapper       apimeta.RESTMapper
	kubeClient   dynamic.Interface
	promClient   prom.Client
	queryBuilder ExternalMetricQueryBuilder

	SeriesRegistry
}

//TODO: It probably makes more sense to, once this is functional and complete, roll the
//prometheusProvider and externalPrometheusProvider up into a single type
//that implements both interfaces or provide a thin wrapper that composes them.
//Just glancing at start.go looks like it would be much more straightforward
//to do one of those two things instead of trying to run the two providers
//independently.
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
	query := p.queryBuilder.BuildPrometheusQuery(namespace, metricName, metricSelector)
	selector := prom.Selector(query)

	//TODO: I don't yet know what a context is, but apparently I should use a real one.
	queryResults, err := p.promClient.Query(context.TODO(), pmodel.Now(), selector)

	//TODO: Only here to stop compiler issues in this incomplete code.
	fmt.Printf("%s, %s", queryResults, err)

	//TODO: Check for errors. See what the custromPrometheusProvider does for errors.
	//TODO: Adapt the results in queryResults to the appropriate return type.
	return nil, nil
}

func (p *externalPrometheusProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	//TODO: Provide a real response.
	return nil
}
