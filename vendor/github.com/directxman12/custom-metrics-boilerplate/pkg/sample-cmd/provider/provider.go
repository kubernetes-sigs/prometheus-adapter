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
	"time"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/client-go/pkg/api"
	_ "k8s.io/client-go/pkg/api/install"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"
)

type incrementalTestingProvider struct {
	client coreclient.CoreV1Interface

	values map[provider.MetricInfo]int64
}

func NewFakeProvider(client coreclient.CoreV1Interface) provider.CustomMetricsProvider {
	return &incrementalTestingProvider{
		client: client,
		values: make(map[provider.MetricInfo]int64),
	}
}

func (p *incrementalTestingProvider) valueFor(groupResource schema.GroupResource, metricName string, namespaced bool) int64 {
	info := provider.MetricInfo{
		GroupResource: groupResource,
		Metric: metricName,
		Namespaced: namespaced,
	}

	value := p.values[info]
	value += 1
	p.values[info] = value

	return value
}

func (p *incrementalTestingProvider) metricFor(value int64, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	group, err := api.Registry.Group(groupResource.Group)
	if err != nil {
		return nil, err
	}
	kind, err := api.Registry.RESTMapper().KindFor(groupResource.WithVersion(group.GroupVersion.Version))
	if err != nil {
		return nil, err
	}

	return &custom_metrics.MetricValue{
		DescribedObject: api.ObjectReference{
			APIVersion: groupResource.Group+"/"+runtime.APIVersionInternal,
			Kind: kind.Kind,
			Name: name,
			Namespace: namespace,
		},
		MetricName: metricName,
		Timestamp: metav1.Time{time.Now()},
		Value: *resource.NewMilliQuantity(value * 100, resource.DecimalSI),
	}, nil
}

func (p *incrementalTestingProvider) metricsFor(totalValue int64, groupResource schema.GroupResource, metricName string, list runtime.Object) (*custom_metrics.MetricValueList, error) {
	if !apimeta.IsListType(list) {
		return nil, fmt.Errorf("returned object was not a list")
	}

	res := make([]custom_metrics.MetricValue, 0)

	err := apimeta.EachListItem(list, func(item runtime.Object) error {
		objMeta := item.(metav1.ObjectMetaAccessor).GetObjectMeta()
		value, err := p.metricFor(0, groupResource, objMeta.GetNamespace(), objMeta.GetName(), metricName)
		if err != nil {
			return err
		}
		res = append(res, *value)

		return nil
	})
	if err != nil {
		return nil, err
	}

	for i := range res {
		res[i].Value = *resource.NewMilliQuantity(100 * totalValue / int64(len(res)), resource.DecimalSI)
	}

	//return p.metricFor(value, groupResource, "", name, metricName)
	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *incrementalTestingProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	value := p.valueFor(groupResource, metricName, false)
	return p.metricFor(value, groupResource, "", name, metricName)
}


func (p *incrementalTestingProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	totalValue := p.valueFor(groupResource, metricName, false)

	// TODO: work for objects not in core v1
	matchingObjectsRaw, err := p.client.RESTClient().Get().
		Resource(groupResource.Resource).
		VersionedParams(&metav1.ListOptions{LabelSelector: selector.String()}, scheme.ParameterCodec).
		Do().
		Get()
	if err != nil {
		return nil, err
	}
	return p.metricsFor(totalValue, groupResource, metricName, matchingObjectsRaw)
}

func (p *incrementalTestingProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	value := p.valueFor(groupResource, metricName, true)
	return p.metricFor(value, groupResource, namespace, name, metricName)
}

func (p *incrementalTestingProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	totalValue := p.valueFor(groupResource, metricName, true)

	// TODO: work for objects not in core v1
	matchingObjectsRaw, err := p.client.RESTClient().Get().
		Namespace(namespace).
		Resource(groupResource.Resource).
		VersionedParams(&metav1.ListOptions{LabelSelector: selector.String()}, scheme.ParameterCodec).
		Do().
		Get()
	if err != nil {
		return nil, err
	}
	return p.metricsFor(totalValue, groupResource, metricName, matchingObjectsRaw)
}

func (p *incrementalTestingProvider) ListAllMetrics() []provider.MetricInfo {
	// TODO: maybe dynamically generate this?
	return []provider.MetricInfo{
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric: "packets-per-second",
			Namespaced: true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "services"},
			Metric: "connections-per-second",
			Namespaced: true,
		},
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "namespaces"},
			Metric: "queue-length",
			Namespaced: false,
		},
	}
}
