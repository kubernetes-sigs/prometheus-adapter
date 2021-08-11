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
	"math"
	"time"

	pmodel "github.com/prometheus/common/model"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider/helpers"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"
)

// Runnable represents something that can be run until told to stop.
type Runnable interface {
	// Run runs the runnable forever.
	Run()
	// RunUntil runs the runnable until the given channel is closed.
	RunUntil(stopChan <-chan struct{})
}

type prometheusProvider struct {
	mapper     apimeta.RESTMapper
	kubeClient dynamic.Interface
	promClient prom.Client

	SeriesRegistry
}

func NewPrometheusProvider(mapper apimeta.RESTMapper, kubeClient dynamic.Interface, promClient prom.Client, namers []naming.MetricNamer, updateInterval time.Duration, maxAge time.Duration) (provider.CustomMetricsProvider, Runnable) {
	lister := &cachingMetricsLister{
		updateInterval: updateInterval,
		maxAge:         maxAge,
		promClient:     promClient,
		namers:         namers,

		SeriesRegistry: &basicSeriesRegistry{
			mapper: mapper,
		},
	}

	return &prometheusProvider{
		mapper:     mapper,
		kubeClient: kubeClient,
		promClient: promClient,

		SeriesRegistry: lister,
	}, lister
}

func (p *prometheusProvider) metricFor(value pmodel.SampleValue, name types.NamespacedName, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	ref, err := helpers.ReferenceFor(p.mapper, name, info)
	if err != nil {
		return nil, err
	}

	var q *resource.Quantity
	if math.IsNaN(float64(value)) {
		q = resource.NewQuantity(0, resource.DecimalSI)
	} else {
		q = resource.NewMilliQuantity(int64(value*1000.0), resource.DecimalSI)
	}

	metric := &custom_metrics.MetricValue{
		DescribedObject: ref,
		Metric: custom_metrics.MetricIdentifier{
			Name: info.Metric,
		},
		// TODO(directxman12): use the right timestamp
		Timestamp: metav1.Time{time.Now()},
		Value:     *q,
	}

	if !metricSelector.Empty() {
		sel, err := metav1.ParseToLabelSelector(metricSelector.String())
		if err != nil {
			return nil, err
		}
		metric.Metric.Selector = sel
	}

	return metric, nil
}

func (p *prometheusProvider) metricsFor(valueSet pmodel.Vector, namespace string, names []string, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValueList, error) {
	values, found := p.MatchValuesToNames(info, valueSet)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}
	res := []custom_metrics.MetricValue{}

	for _, name := range names {
		if _, found := values[name]; !found {
			continue
		}

		value, err := p.metricFor(values[name], types.NamespacedName{Namespace: namespace, Name: name}, info, metricSelector)
		if err != nil {
			return nil, err
		}
		res = append(res, *value)
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *prometheusProvider) buildQuery(ctx context.Context, info provider.CustomMetricInfo, namespace string, metricSelector labels.Selector, names ...string) (pmodel.Vector, error) {
	query, found := p.QueryForMetric(info, namespace, metricSelector, names...)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}

	// TODO: use an actual context
	queryResults, err := p.promClient.Query(ctx, pmodel.Now(), query)
	if err != nil {
		klog.Errorf("unable to fetch metrics from prometheus: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	if queryResults.Type != pmodel.ValVector {
		klog.Errorf("unexpected results from prometheus: expected %s, got %s on results %v", pmodel.ValVector, queryResults.Type, queryResults)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	return *queryResults.Vector, nil
}

func (p *prometheusProvider) GetMetricByName(ctx context.Context, name types.NamespacedName, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValue, error) {
	// construct a query
	queryResults, err := p.buildQuery(ctx, info, name.Namespace, metricSelector, name.Name)
	if err != nil {
		return nil, err
	}

	// associate the metrics
	if len(queryResults) < 1 {
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name.Name)
	}

	namedValues, found := p.MatchValuesToNames(info, queryResults)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}

	if len(namedValues) > 1 {
		klog.V(2).Infof("Got more than one result (%v results) when fetching metric %s for %q, using the first one with a matching name...", len(queryResults), info.String(), name)
	}

	resultValue, nameFound := namedValues[name.Name]
	if !nameFound {
		klog.Errorf("None of the results returned by when fetching metric %s for %q matched the resource name", info.String(), name)
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name.Name)
	}

	// return the resulting metric
	return p.metricFor(resultValue, name, info, metricSelector)
}

func (p *prometheusProvider) GetMetricBySelector(ctx context.Context, namespace string, selector labels.Selector, info provider.CustomMetricInfo, metricSelector labels.Selector) (*custom_metrics.MetricValueList, error) {
	// fetch a list of relevant resource names
	resourceNames, err := helpers.ListObjectNames(p.mapper, p.kubeClient, namespace, selector, info)
	if err != nil {
		klog.Errorf("unable to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}

	// construct the actual query
	queryResults, err := p.buildQuery(ctx, info, namespace, metricSelector, resourceNames...)
	if err != nil {
		return nil, err
	}

	// return the resulting metrics
	return p.metricsFor(queryResults, namespace, resourceNames, info, metricSelector)
}

type cachingMetricsLister struct {
	SeriesRegistry

	promClient     prom.Client
	updateInterval time.Duration
	maxAge         time.Duration
	namers         []naming.MetricNamer
}

func (l *cachingMetricsLister) Run() {
	l.RunUntil(wait.NeverStop)
}

func (l *cachingMetricsLister) RunUntil(stopChan <-chan struct{}) {
	go wait.Until(func() {
		if err := l.updateMetrics(); err != nil {
			utilruntime.HandleError(err)
		}
	}, l.updateInterval, stopChan)
}

type selectorSeries struct {
	selector prom.Selector
	series   []prom.Series
}

func (l *cachingMetricsLister) updateMetrics() error {
	startTime := pmodel.Now().Add(-1 * l.maxAge)

	// don't do duplicate queries when it's just the matchers that change
	seriesCacheByQuery := make(map[prom.Selector][]prom.Series)

	// these can take a while on large clusters, so launch in parallel
	// and don't duplicate
	selectors := make(map[prom.Selector]struct{})
	selectorSeriesChan := make(chan selectorSeries, len(l.namers))
	errs := make(chan error, len(l.namers))
	for _, namer := range l.namers {
		sel := namer.Selector()
		if _, ok := selectors[sel]; ok {
			errs <- nil
			selectorSeriesChan <- selectorSeries{}
			continue
		}
		selectors[sel] = struct{}{}
		go func() {
			series, err := l.promClient.Series(context.TODO(), pmodel.Interval{startTime, 0}, sel)
			if err != nil {
				errs <- fmt.Errorf("unable to fetch metrics for query %q: %v", sel, err)
				return
			}
			errs <- nil
			selectorSeriesChan <- selectorSeries{
				selector: sel,
				series:   series,
			}
		}()
	}

	// iterate through, blocking until we've got all results
	for range l.namers {
		if err := <-errs; err != nil {
			return fmt.Errorf("unable to update list of all metrics: %v", err)
		}
		if ss := <-selectorSeriesChan; ss.series != nil {
			seriesCacheByQuery[ss.selector] = ss.series
		}
	}
	close(errs)

	newSeries := make([][]prom.Series, len(l.namers))
	for i, namer := range l.namers {
		series, cached := seriesCacheByQuery[namer.Selector()]
		if !cached {
			return fmt.Errorf("unable to update list of all metrics: no metrics retrieved for query %q", namer.Selector())
		}
		newSeries[i] = namer.FilterSeries(series)
	}

	klog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)

	return l.SetSeries(newSeries, l.namers)
}
