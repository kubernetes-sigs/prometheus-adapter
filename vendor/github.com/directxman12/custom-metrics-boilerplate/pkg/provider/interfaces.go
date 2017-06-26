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

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

// MetricInfo describes a metric for a particular
// fully-qualified group resource.
type MetricInfo struct {
	GroupResource schema.GroupResource
	Namespaced    bool
	Metric        string
}

func (i MetricInfo) String() string {
	if i.Namespaced {
		return fmt.Sprintf("%s/%s(namespaced)", i.GroupResource.String(), i.Metric)
	} else {
		return fmt.Sprintf("%s/%s", i.GroupResource.String(), i.Metric)
	}
}

// CustomMetricsProvider is a soruce of custom metrics
// which is able to supply a list of available metrics,
// as well as metric values themselves on demand.
//
// Note that group-resources are provided  as GroupResources,
// not GroupKinds.  This is to allow flexibility on the part
// of the implementor: implementors do not necessarily need
// to be aware of all existing kinds and their corresponding
// REST mappings in order to perform queries.
//
// For queries that use label selectors, it is up to the
// implementor to decide how to make use of the label selector --
// they may wish to query the main Kubernetes API server, or may
// wish to simply make use of stored information in their TSDB.
type CustomMetricsProvider interface {
	// GetRootScopedMetricByName fetches a particular metric for a particular root-scoped object.
	GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error)

	// GetRootScopedMetricByName fetches a particular metric for a set of root-scoped objects
	// matching the given label selector.
	GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error)

	// GetNamespacedMetricByName fetches a particular metric for a particular namespaced object.
	GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error)

	// GetNamespacedMetricByName fetches a particular metric for a set of namespaced objects
	// matching the given label selector.
	GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error)

	// ListAllMetrics provides a list of all available metrics at
	// the current time.  Note that this is not allowed to return
	// an error, so it is reccomended that implementors cache and
	// periodically update this list, instead of querying every time.
	ListAllMetrics() []MetricInfo
}
