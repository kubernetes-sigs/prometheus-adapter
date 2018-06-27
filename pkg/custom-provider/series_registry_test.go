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
	"sort"
	"testing"
	"time"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	pmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coreapi "k8s.io/api/core/v1"
	extapi "k8s.io/api/extensions/v1beta1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	config "github.com/directxman12/k8s-prometheus-adapter/cmd/config-gen/utils"
	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// restMapper creates a RESTMapper with just the types we need for
// these tests.
func restMapper() apimeta.RESTMapper {
	mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{coreapi.SchemeGroupVersion})

	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Pod"), apimeta.RESTScopeNamespace)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Service"), apimeta.RESTScopeNamespace)
	mapper.Add(extapi.SchemeGroupVersion.WithKind("Ingress"), apimeta.RESTScopeNamespace)
	mapper.Add(extapi.SchemeGroupVersion.WithKind("Deployment"), apimeta.RESTScopeNamespace)

	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Node"), apimeta.RESTScopeRoot)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("PersistentVolume"), apimeta.RESTScopeRoot)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Namespace"), apimeta.RESTScopeRoot)

	return mapper
}

func setupMetricNamer(t testing.TB) []MetricNamer {
	cfg := config.DefaultConfig(1*time.Minute, "kube_")
	namers, err := NamersFromConfig(cfg, restMapper())
	require.NoError(t, err)
	return namers
}

var seriesRegistryTestSeries = [][]prom.Series{
	// container series
	{
		{
			Name:   "container_some_time_seconds_total",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
	},
	{
		{
			Name:   "container_some_count_total",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
	},
	{
		{
			Name:   "container_some_usage",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
	},
	{
		// guage metrics
		{
			Name:   "node_gigawatts",
			Labels: pmodel.LabelSet{"kube_node": "somenode"},
		},
		{
			Name:   "service_proxy_packets",
			Labels: pmodel.LabelSet{"kube_service": "somesvc", "kube_namespace": "somens"},
		},
	},
	{
		// cumulative --> rate metrics
		{
			Name:   "ingress_hits_total",
			Labels: pmodel.LabelSet{"kube_ingress": "someingress", "kube_service": "somesvc", "kube_pod": "backend1", "kube_namespace": "somens"},
		},
		{
			Name:   "ingress_hits_total",
			Labels: pmodel.LabelSet{"kube_ingress": "someingress", "kube_service": "somesvc", "kube_pod": "backend2", "kube_namespace": "somens"},
		},
		{
			Name:   "volume_claims_total",
			Labels: pmodel.LabelSet{"kube_persistentvolume": "somepv"},
		},
	},
	{
		// cumulative seconds --> rate metrics
		{
			Name:   "work_queue_wait_seconds_total",
			Labels: pmodel.LabelSet{"kube_deployment": "somedep", "kube_namespace": "somens"},
		},
		{
			Name:   "node_fan_seconds_total",
			Labels: pmodel.LabelSet{"kube_node": "somenode"},
		},
	},
}

func TestSeriesRegistry(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	namers := setupMetricNamer(t)
	registry := &basicSeriesRegistry{
		mapper: restMapper(),
	}

	// set up the registry
	require.NoError(registry.SetSeries(seriesRegistryTestSeries, namers))

	// make sure each metric got registered and can form queries
	testCases := []struct {
		title         string
		info          provider.CustomMetricInfo
		namespace     string
		resourceNames []string

		expectedQuery string
	}{
		// container metrics
		{
			title:         "container metrics gauge / multiple resource names",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedQuery: "sum(container_some_usage{namespace=\"somens\",pod_name=~\"somepod1|somepod2\",container_name!=\"POD\"}) by (pod_name)",
		},
		{
			title:         "container metrics counter",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedQuery: "sum(rate(container_some_count_total{namespace=\"somens\",pod_name=~\"somepod1|somepod2\",container_name!=\"POD\"}[1m])) by (pod_name)",
		},
		{
			title:         "container metrics seconds counter",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedQuery: "sum(rate(container_some_time_seconds_total{namespace=\"somens\",pod_name=~\"somepod1|somepod2\",container_name!=\"POD\"}[1m])) by (pod_name)",
		},
		// namespaced metrics
		{
			title:         "namespaced metrics counter / multidimensional (service)",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "service"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"somesvc"},

			expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_service=\"somesvc\"}[1m])) by (kube_service)",
		},
		{
			title:         "namespaced metrics counter / multidimensional (ingress)",
			info:          provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingress"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"someingress"},

			expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_ingress=\"someingress\"}[1m])) by (kube_ingress)",
		},
		{
			title:         "namespaced metrics counter / multidimensional (pod)",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "pod"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"somepod"},

			expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_pod=\"somepod\"}[1m])) by (kube_pod)",
		},
		{
			title:         "namespaced metrics gauge",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "service"}, true, "service_proxy_packets"},
			namespace:     "somens",
			resourceNames: []string{"somesvc"},

			expectedQuery: "sum(service_proxy_packets{kube_namespace=\"somens\",kube_service=\"somesvc\"}) by (kube_service)",
		},
		{
			title:         "namespaced metrics seconds counter",
			info:          provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployment"}, true, "work_queue_wait"},
			namespace:     "somens",
			resourceNames: []string{"somedep"},

			expectedQuery: "sum(rate(work_queue_wait_seconds_total{kube_namespace=\"somens\",kube_deployment=\"somedep\"}[1m])) by (kube_deployment)",
		},
		// non-namespaced series
		{
			title:         "root scoped metrics gauge",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "node"}, false, "node_gigawatts"},
			resourceNames: []string{"somenode"},

			expectedQuery: "sum(node_gigawatts{kube_node=\"somenode\"}) by (kube_node)",
		},
		{
			title:         "root scoped metrics counter",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "persistentvolume"}, false, "volume_claims"},
			resourceNames: []string{"somepv"},

			expectedQuery: "sum(rate(volume_claims_total{kube_persistentvolume=\"somepv\"}[1m])) by (kube_persistentvolume)",
		},
		{
			title:         "root scoped metrics seconds counter",
			info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "node"}, false, "node_fan"},
			resourceNames: []string{"somenode"},

			expectedQuery: "sum(rate(node_fan_seconds_total{kube_node=\"somenode\"}[1m])) by (kube_node)",
		},
	}

	for _, testCase := range testCases {
		outputQuery, found := registry.QueryForMetric(testCase.info, testCase.namespace, testCase.resourceNames...)
		if !assert.True(found, "%s: metric %v should available", testCase.title, testCase.info) {
			continue
		}

		assert.Equal(prom.Selector(testCase.expectedQuery), outputQuery, "%s: metric %v should have produced the correct query for %v in namespace %s", testCase.title, testCase.info, testCase.resourceNames, testCase.namespace)
	}

	allMetrics := registry.ListAllMetrics()
	expectedMetrics := []provider.CustomMetricInfo{
		{schema.GroupResource{Resource: "pods"}, true, "some_count"},
		{schema.GroupResource{Resource: "namespaces"}, false, "some_count"},
		{schema.GroupResource{Resource: "pods"}, true, "some_time"},
		{schema.GroupResource{Resource: "namespaces"}, false, "some_time"},
		{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
		{schema.GroupResource{Resource: "namespaces"}, false, "some_usage"},
		{schema.GroupResource{Resource: "services"}, true, "ingress_hits"},
		{schema.GroupResource{Group: "extensions", Resource: "ingresses"}, true, "ingress_hits"},
		{schema.GroupResource{Resource: "pods"}, true, "ingress_hits"},
		{schema.GroupResource{Resource: "namespaces"}, false, "ingress_hits"},
		{schema.GroupResource{Resource: "services"}, true, "service_proxy_packets"},
		{schema.GroupResource{Resource: "namespaces"}, false, "service_proxy_packets"},
		{schema.GroupResource{Group: "extensions", Resource: "deployments"}, true, "work_queue_wait"},
		{schema.GroupResource{Resource: "namespaces"}, false, "work_queue_wait"},
		{schema.GroupResource{Resource: "nodes"}, false, "node_gigawatts"},
		{schema.GroupResource{Resource: "persistentvolumes"}, false, "volume_claims"},
		{schema.GroupResource{Resource: "nodes"}, false, "node_fan"},
	}

	// sort both for easy comparison
	sort.Sort(metricInfoSorter(allMetrics))
	sort.Sort(metricInfoSorter(expectedMetrics))

	assert.Equal(expectedMetrics, allMetrics, "should have listed all expected metrics")
}

func BenchmarkSetSeries(b *testing.B) {
	namers := setupMetricNamer(b)
	registry := &basicSeriesRegistry{
		mapper: restMapper(),
	}

	numDuplicates := 10000
	newSeriesSlices := make([][]prom.Series, len(seriesRegistryTestSeries))
	for i, seriesSlice := range seriesRegistryTestSeries {
		newSlice := make([]prom.Series, len(seriesSlice)*numDuplicates)
		for j, series := range seriesSlice {
			for k := 0; k < numDuplicates; k++ {
				newSlice[j*numDuplicates+k] = series
			}
		}
		newSeriesSlices[i] = newSlice
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.SetSeries(newSeriesSlices, namers)
	}
}

// metricInfoSorter is a sort.Interface for sorting provider.CustomMetricInfos
type metricInfoSorter []provider.CustomMetricInfo

func (s metricInfoSorter) Len() int {
	return len(s)
}

func (s metricInfoSorter) Less(i, j int) bool {
	infoI := s[i]
	infoJ := s[j]

	if infoI.Metric == infoJ.Metric {
		if infoI.GroupResource == infoJ.GroupResource {
			return infoI.Namespaced
		}

		if infoI.GroupResource.Group == infoJ.GroupResource.Group {
			return infoI.GroupResource.Resource < infoJ.GroupResource.Resource
		}

		return infoI.GroupResource.Group < infoJ.GroupResource.Group
	}

	return infoI.Metric < infoJ.Metric
}

func (s metricInfoSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
