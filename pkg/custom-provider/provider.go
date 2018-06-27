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

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	pmodel "github.com/prometheus/common/model"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
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

func NewPrometheusProvider(mapper apimeta.RESTMapper, kubeClient dynamic.Interface, promClient prom.Client, namers []MetricNamer, updateInterval time.Duration) (provider.CustomMetricsProvider, Runnable) {
	lister := &cachingMetricsLister{
		updateInterval: updateInterval,
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

func (p *prometheusProvider) metricFor(value pmodel.SampleValue, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	kind, err := p.mapper.KindFor(groupResource.WithVersion(""))
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: custom_metrics.ObjectReference{
			APIVersion: groupResource.Group + "/" + runtime.APIVersionInternal,
			Kind:       kind.Kind,
			Name:       name,
			Namespace:  namespace,
		},
		MetricName: metricName,
		Timestamp:  metav1.Time{time.Now()},
		Value:      *resource.NewMilliQuantity(int64(value*1000.0), resource.DecimalSI),
	}, nil
}

func (p *prometheusProvider) metricsFor(valueSet pmodel.Vector, info provider.CustomMetricInfo, list runtime.Object) (*custom_metrics.MetricValueList, error) {
	if !apimeta.IsListType(list) {
		return nil, apierr.NewInternalError(fmt.Errorf("result of label selector list operation was not a list"))
	}

	values, found := p.MatchValuesToNames(info, valueSet)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}
	res := []custom_metrics.MetricValue{}

	err := apimeta.EachListItem(list, func(item runtime.Object) error {
		objUnstructured := item.(*unstructured.Unstructured)
		objName := objUnstructured.GetName()
		if _, found := values[objName]; !found {
			return nil
		}
		value, err := p.metricFor(values[objName], info.GroupResource, objUnstructured.GetNamespace(), objName, info.Metric)
		if err != nil {
			return err
		}
		res = append(res, *value)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *prometheusProvider) buildQuery(info provider.CustomMetricInfo, namespace string, names ...string) (pmodel.Vector, error) {
	query, found := p.QueryForMetric(info, namespace, names...)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}

	// TODO: use an actual context
	queryResults, err := p.promClient.Query(context.TODO(), pmodel.Now(), query)
	if err != nil {
		glog.Errorf("unable to fetch metrics from prometheus: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	if queryResults.Type != pmodel.ValVector {
		glog.Errorf("unexpected results from prometheus: expected %s, got %s on results %v", pmodel.ValVector, queryResults.Type, queryResults)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	return *queryResults.Vector, nil
}

func (p *prometheusProvider) getSingle(info provider.CustomMetricInfo, namespace, name string) (*custom_metrics.MetricValue, error) {
	queryResults, err := p.buildQuery(info, namespace, name)
	if err != nil {
		return nil, err
	}

	if len(queryResults) < 1 {
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name)
	}

	namedValues, found := p.MatchValuesToNames(info, queryResults)
	if !found {
		return nil, provider.NewMetricNotFoundError(info.GroupResource, info.Metric)
	}

	if len(namedValues) > 1 {
		glog.V(2).Infof("Got more than one result (%v results) when fetching metric %s for %q, using the first one with a matching name...", len(queryResults), info.String(), name)
	}

	resultValue, nameFound := namedValues[name]
	if !nameFound {
		glog.Errorf("None of the results returned by when fetching metric %s for %q matched the resource name", info.String(), name)
		return nil, provider.NewMetricNotFoundForError(info.GroupResource, info.Metric, name)
	}

	return p.metricFor(resultValue, info.GroupResource, "", name, info.Metric)
}

func (p *prometheusProvider) getMultiple(info provider.CustomMetricInfo, namespace string, selector labels.Selector) (*custom_metrics.MetricValueList, error) {
	fullResources, err := p.mapper.ResourcesFor(info.GroupResource.WithVersion(""))
	if err == nil && len(fullResources) == 0 {
		err = fmt.Errorf("no fully versioned resources known for group-resource %v", info.GroupResource)
	}
	if err != nil {
		glog.Errorf("unable to find preferred version to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}
	var client dynamic.ResourceInterface
	if namespace != "" {
		client = p.kubeClient.Resource(fullResources[0]).Namespace(namespace)
	} else {
		client = p.kubeClient.Resource(fullResources[0])
	}

	// actually list the objects matching the label selector
	matchingObjectsRaw, err := client.List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		glog.Errorf("unable to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}

	// make sure we have a list
	if !apimeta.IsListType(matchingObjectsRaw) {
		return nil, apierr.NewInternalError(fmt.Errorf("result of label selector list operation was not a list"))
	}

	// convert a list of objects into the corresponding list of names
	resourceNames := []string{}
	err = apimeta.EachListItem(matchingObjectsRaw, func(item runtime.Object) error {
		objName := item.(*unstructured.Unstructured).GetName()
		resourceNames = append(resourceNames, objName)
		return nil
	})

	// construct the actual query
	queryResults, err := p.buildQuery(info, namespace, resourceNames...)
	if err != nil {
		return nil, err
	}
	return p.metricsFor(queryResults, info, matchingObjectsRaw)
}

func (p *prometheusProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}

	return p.getSingle(info, "", name)
}

func (p *prometheusProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}
	return p.getMultiple(info, "", selector)
}

func (p *prometheusProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    true,
	}

	return p.getSingle(info, namespace, name)
}

func (p *prometheusProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	info := provider.CustomMetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    true,
	}
	return p.getMultiple(info, namespace, selector)
}

type cachingMetricsLister struct {
	SeriesRegistry

	promClient     prom.Client
	updateInterval time.Duration
	namers         []MetricNamer
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
	startTime := pmodel.Now().Add(-1 * l.updateInterval)

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

	glog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)

	return l.SetSeries(newSeries, l.namers)
}
