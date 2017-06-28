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
	"github.com/golang/glog"
	"net/http"
	"time"

	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"
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
	"k8s.io/client-go/pkg/api"
	_ "k8s.io/client-go/pkg/api/install"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// newMetricNotFoundError returns a StatusError indicating the given metric could not be found.
// It is similar to NewNotFound, but more specialized
func newMetricNotFoundError(resource schema.GroupResource, metricName string) *apierr.StatusError {
	return &apierr.StatusError{metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    int32(http.StatusNotFound),
		Reason:  metav1.StatusReasonNotFound,
		Message: fmt.Sprintf("the server could not find the metric %s for %s", metricName, resource.String()),
	}}
}

// newMetricNotFoundForError returns a StatusError indicating the given metric could not be found for
// the given named object. It is similar to NewNotFound, but more specialized
func newMetricNotFoundForError(resource schema.GroupResource, metricName string, resourceName string) *apierr.StatusError {
	return &apierr.StatusError{metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    int32(http.StatusNotFound),
		Reason:  metav1.StatusReasonNotFound,
		Message: fmt.Sprintf("the server could not find the metric %s for %s %s", metricName, resource.String(), resourceName),
	}}
}

type prometheusProvider struct {
	mapper     apimeta.RESTMapper
	kubeClient dynamic.ClientPool
	promClient prom.Client

	SeriesRegistry

	rateInterval time.Duration
}

func NewPrometheusProvider(mapper apimeta.RESTMapper, kubeClient dynamic.ClientPool, promClient prom.Client, updateInterval time.Duration, rateInterval time.Duration, stopChan <-chan struct{}) provider.CustomMetricsProvider {
	lister := &cachingMetricsLister{
		updateInterval: updateInterval,
		promClient:     promClient,

		SeriesRegistry: &basicSeriesRegistry{
			namer: metricNamer{
				// TODO: populate the overrides list
				overrides: nil,
				mapper:    mapper,
			},
		},
	}

	lister.RunUntil(stopChan)

	return &prometheusProvider{
		mapper:     mapper,
		kubeClient: kubeClient,
		promClient: promClient,

		SeriesRegistry: lister,

		rateInterval: rateInterval,
	}
}

func (p *prometheusProvider) metricFor(value pmodel.SampleValue, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	kind, err := p.mapper.KindFor(groupResource.WithVersion(""))
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: api.ObjectReference{
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

func (p *prometheusProvider) metricsFor(valueSet pmodel.Vector, info provider.MetricInfo, list runtime.Object) (*custom_metrics.MetricValueList, error) {
	if !apimeta.IsListType(list) {
		// TODO: fix the error type here
		return nil, fmt.Errorf("returned object was not a list")
	}

	values, found := p.MatchValuesToNames(info, valueSet)
	if !found {
		// TODO: throw error
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

func (p *prometheusProvider) buildQuery(info provider.MetricInfo, namespace string, names ...string) (pmodel.Vector, error) {
	kind, baseQuery, groupBy, found := p.QueryForMetric(info, namespace, names...)
	if !found {
		return nil, newMetricNotFoundError(info.GroupResource, info.Metric)
	}

	fullQuery := baseQuery
	switch kind {
	case CounterSeries:
		fullQuery = prom.Selector(fmt.Sprintf("rate(%s[%s])", baseQuery, pmodel.Duration(p.rateInterval).String()))
	case SecondsCounterSeries:
		// TODO: futher modify for seconds?
		fullQuery = prom.Selector(prom.Selector(fmt.Sprintf("rate(%s[%s])", baseQuery, pmodel.Duration(p.rateInterval).String())))
	}

	// NB: too small of a rate interval will return no results...

	// sum over all other dimensions of this query (e.g. if we select on route, sum across all pods,
	// but if we select on pods, sum across all routes), and split by the dimension of our resource
	// TODO: return/populate the by list in SeriesForMetric
	fullQuery = prom.Selector(fmt.Sprintf("sum(%s) by (%s)", fullQuery, groupBy))

	// TODO: use an actual context
	queryResults, err := p.promClient.Query(context.Background(), pmodel.Now(), fullQuery)
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

func (p *prometheusProvider) getSingle(info provider.MetricInfo, namespace, name string) (*custom_metrics.MetricValue, error) {
	queryResults, err := p.buildQuery(info, namespace, name)
	if err != nil {
		return nil, err
	}

	if len(queryResults) < 1 {
		return nil, newMetricNotFoundForError(info.GroupResource, info.Metric, name)
	}
	// TODO: check if lenght of results > 1?
	// TODO: check if our output name is the same as our input name
	resultValue := queryResults[0].Value
	return p.metricFor(resultValue, info.GroupResource, "", name, info.Metric)
}

func (p *prometheusProvider) getMultiple(info provider.MetricInfo, namespace string, selector labels.Selector) (*custom_metrics.MetricValueList, error) {
	// construct a client to list the names of objects matching the label selector
	client, err := p.kubeClient.ClientForGroupVersionResource(info.GroupResource.WithVersion(""))
	if err != nil {
		glog.Errorf("unable to construct dynamic client to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}

	// we can construct a this APIResource ourself, since the dynamic client only uses Name and Namespaced
	apiRes := &metav1.APIResource{
		Name:       info.GroupResource.Resource,
		Namespaced: info.Namespaced,
	}

	// actually list the objects matching the label selector
	matchingObjectsRaw, err := client.Resource(apiRes, namespace).
		List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		glog.Errorf("unable to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}

	// make sure we have a list
	if !apimeta.IsListType(matchingObjectsRaw) {
		// TODO: fix the error type here
		return nil, fmt.Errorf("returned object was not a list")
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
	info := provider.MetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}

	return p.getSingle(info, "", name)
}

func (p *prometheusProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	info := provider.MetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    false,
	}
	return p.getMultiple(info, "", selector)
}

func (p *prometheusProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	info := provider.MetricInfo{
		GroupResource: groupResource,
		Metric:        metricName,
		Namespaced:    true,
	}

	return p.getSingle(info, namespace, name)
}

func (p *prometheusProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	info := provider.MetricInfo{
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

func (l *cachingMetricsLister) updateMetrics() error {
	startTime := pmodel.Now().Add(-1 * l.updateInterval)

	// container-specific metrics from cAdvsior have their own form, and need special handling
	containerSel := prom.MatchSeries("", prom.NameMatches("^container_.*"), prom.LabelNeq("container_name", "POD"), prom.LabelNeq("namespace", ""), prom.LabelNeq("pod_name", ""))
	namespacedSel := prom.MatchSeries("", prom.LabelNeq("namespace", ""), prom.NameNotMatches("^container_.*"))
	// TODO: figure out how to determine which metrics on non-namespaced objects are kubernetes-related

	// TODO: use an actual context here
	series, err := l.promClient.Series(context.Background(), pmodel.Interval{startTime, 0}, containerSel, namespacedSel)
	if err != nil {
		return fmt.Errorf("unable to update list of all available metrics: %v", err)
	}

	glog.V(10).Infof("Set available metric list from Prometheus to: %v", series)

	l.SetSeries(series)

	return nil
}
