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

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	pmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coreapi "k8s.io/api/core/v1"
	extapi "k8s.io/api/extensions/v1beta1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// restMapper creates a RESTMapper with just the types we need for
// these tests.
func restMapper() apimeta.RESTMapper {
	mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{coreapi.SchemeGroupVersion}, apimeta.InterfacesForUnstructured)

	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Pod"), apimeta.RESTScopeNamespace)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Service"), apimeta.RESTScopeNamespace)
	mapper.Add(extapi.SchemeGroupVersion.WithKind("Ingress"), apimeta.RESTScopeNamespace)
	mapper.Add(extapi.SchemeGroupVersion.WithKind("Deployment"), apimeta.RESTScopeNamespace)

	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Node"), apimeta.RESTScopeRoot)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("PersistentVolume"), apimeta.RESTScopeRoot)
	mapper.Add(coreapi.SchemeGroupVersion.WithKind("Namespace"), apimeta.RESTScopeRoot)

	return mapper
}

func setupMetricNamer(t *testing.T) *metricNamer {
	return &metricNamer{
		overrides: map[string]seriesSpec{
			"container_actually_gauge_seconds_total": {
				metricName: "actually_gauge",
				kind:       GaugeSeries,
			},
		},
		labelPrefix: "kube_",
		mapper:      restMapper(),
	}
}

func TestMetricNamerContainerSeries(t *testing.T) {
	testCases := []struct {
		input            prom.Series
		outputMetricName string
		outputInfo       seriesInfo
	}{
		{
			input: prom.Series{
				Name:   "container_actually_gauge_seconds_total",
				Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "actually_gauge",
			outputInfo: seriesInfo{
				baseSeries:  prom.Series{Name: "container_actually_gauge_seconds_total"},
				kind:        GaugeSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name:   "container_some_usage",
				Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_usage",
			outputInfo: seriesInfo{
				baseSeries:  prom.Series{Name: "container_some_usage"},
				kind:        GaugeSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name:   "container_some_count_total",
				Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_count",
			outputInfo: seriesInfo{
				baseSeries:  prom.Series{Name: "container_some_count_total"},
				kind:        CounterSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name:   "container_some_time_seconds_total",
				Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_time",
			outputInfo: seriesInfo{
				baseSeries:  prom.Series{Name: "container_some_time_seconds_total"},
				kind:        SecondsCounterSeries,
				isContainer: true,
			},
		},
	}

	assert := assert.New(t)

	namer := setupMetricNamer(t)
	resMap := map[provider.MetricInfo]seriesInfo{}

	for _, test := range testCases {
		namer.processContainerSeries(test.input, resMap)
		metric := provider.MetricInfo{
			Metric:        test.outputMetricName,
			GroupResource: schema.GroupResource{Resource: "pods"},
			Namespaced:    true,
		}
		if assert.Contains(resMap, metric) {
			assert.Equal(test.outputInfo, resMap[metric])
		}
	}
}

func TestSeriesRegistry(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	namer := setupMetricNamer(t)
	registry := &basicSeriesRegistry{
		namer: *namer,
	}

	inputSeries := []prom.Series{
		// container series
		{
			Name:   "container_actually_gauge_seconds_total",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name:   "container_some_usage",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name:   "container_some_count_total",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name:   "container_some_time_seconds_total",
			Labels: pmodel.LabelSet{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		// namespaced series
		// a series that should turn into multiple metrics
		{
			Name:   "ingress_hits_total",
			Labels: pmodel.LabelSet{"kube_ingress": "someingress", "kube_service": "somesvc", "kube_pod": "backend1", "kube_namespace": "somens"},
		},
		{
			Name:   "ingress_hits_total",
			Labels: pmodel.LabelSet{"kube_ingress": "someingress", "kube_service": "somesvc", "kube_pod": "backend2", "kube_namespace": "somens"},
		},
		{
			Name:   "service_proxy_packets",
			Labels: pmodel.LabelSet{"kube_service": "somesvc", "kube_namespace": "somens"},
		},
		{
			Name:   "work_queue_wait_seconds_total",
			Labels: pmodel.LabelSet{"kube_deployment": "somedep", "kube_namespace": "somens"},
		},
		// non-namespaced series
		{
			Name:   "node_gigawatts",
			Labels: pmodel.LabelSet{"kube_node": "somenode"},
		},
		{
			Name:   "volume_claims_total",
			Labels: pmodel.LabelSet{"kube_persistentvolume": "somepv"},
		},
		{
			Name:   "node_fan_seconds_total",
			Labels: pmodel.LabelSet{"kube_node": "somenode"},
		},
		// unrelated series
		{
			Name:   "admin_coffee_liters_total",
			Labels: pmodel.LabelSet{"admin": "some-admin"},
		},
		{
			Name:   "admin_unread_emails",
			Labels: pmodel.LabelSet{"admin": "some-admin"},
		},
		{
			Name:   "admin_reddit_seconds_total",
			Labels: pmodel.LabelSet{"kube_admin": "some-admin"},
		},
	}

	// set up the registry
	require.NoError(registry.SetSeries(inputSeries))

	// make sure each metric got registered and can form queries
	testCases := []struct {
		title         string
		info          provider.MetricInfo
		namespace     string
		resourceNames []string

		expectedKind    SeriesType
		expectedQuery   string
		expectedGroupBy string
	}{
		// container metrics
		{
			title:         "container metrics overrides / single resource name",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "actually_gauge"},
			namespace:     "somens",
			resourceNames: []string{"somepod"},

			expectedKind:    GaugeSeries,
			expectedQuery:   "container_actually_gauge_seconds_total{pod_name=\"somepod\",container_name!=\"POD\",namespace=\"somens\"}",
			expectedGroupBy: "pod_name",
		},
		{
			title:         "container metrics gauge / multiple resource names",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind:    GaugeSeries,
			expectedQuery:   "container_some_usage{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
			expectedGroupBy: "pod_name",
		},
		{
			title:         "container metrics counter",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind:    CounterSeries,
			expectedQuery:   "container_some_count_total{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
			expectedGroupBy: "pod_name",
		},
		{
			title:         "container metrics seconds counter",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
			namespace:     "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind:    SecondsCounterSeries,
			expectedQuery:   "container_some_time_seconds_total{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
			expectedGroupBy: "pod_name",
		},
		// namespaced metrics
		{
			title:         "namespaced metrics counter / multidimensional (service)",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "service"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"somesvc"},

			expectedKind:  CounterSeries,
			expectedQuery: "ingress_hits_total{kube_service=\"somesvc\",kube_namespace=\"somens\"}",
		},
		{
			title:         "namespaced metrics counter / multidimensional (ingress)",
			info:          provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingress"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"someingress"},

			expectedKind:  CounterSeries,
			expectedQuery: "ingress_hits_total{kube_ingress=\"someingress\",kube_namespace=\"somens\"}",
		},
		{
			title:         "namespaced metrics counter / multidimensional (pod)",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "pod"}, true, "ingress_hits"},
			namespace:     "somens",
			resourceNames: []string{"somepod"},

			expectedKind:  CounterSeries,
			expectedQuery: "ingress_hits_total{kube_pod=\"somepod\",kube_namespace=\"somens\"}",
		},
		{
			title:         "namespaced metrics gauge",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "service"}, true, "service_proxy_packets"},
			namespace:     "somens",
			resourceNames: []string{"somesvc"},

			expectedKind:  GaugeSeries,
			expectedQuery: "service_proxy_packets{kube_service=\"somesvc\",kube_namespace=\"somens\"}",
		},
		{
			title:         "namespaced metrics seconds counter",
			info:          provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployment"}, true, "work_queue_wait"},
			namespace:     "somens",
			resourceNames: []string{"somedep"},

			expectedKind:  SecondsCounterSeries,
			expectedQuery: "work_queue_wait_seconds_total{kube_deployment=\"somedep\",kube_namespace=\"somens\"}",
		},
		// non-namespaced series
		{
			title:         "root scoped metrics gauge",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "node"}, false, "node_gigawatts"},
			resourceNames: []string{"somenode"},

			expectedKind:  GaugeSeries,
			expectedQuery: "node_gigawatts{kube_node=\"somenode\"}",
		},
		{
			title:         "root scoped metrics counter",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "persistentvolume"}, false, "volume_claims"},
			resourceNames: []string{"somepv"},

			expectedKind:  CounterSeries,
			expectedQuery: "volume_claims_total{kube_persistentvolume=\"somepv\"}",
		},
		{
			title:         "root scoped metrics seconds counter",
			info:          provider.MetricInfo{schema.GroupResource{Resource: "node"}, false, "node_fan"},
			resourceNames: []string{"somenode"},

			expectedKind:  SecondsCounterSeries,
			expectedQuery: "node_fan_seconds_total{kube_node=\"somenode\"}",
		},
	}

	for _, testCase := range testCases {
		outputKind, outputQuery, groupBy, found := registry.QueryForMetric(testCase.info, testCase.namespace, testCase.resourceNames...)
		if !assert.True(found, "%s: metric %v should available", testCase.title, testCase.info) {
			continue
		}

		assert.Equal(testCase.expectedKind, outputKind, "%s: metric %v should have had the right series type", testCase.title, testCase.info)
		assert.Equal(prom.Selector(testCase.expectedQuery), outputQuery, "%s: metric %v should have produced the correct query for %v in namespace %s", testCase.title, testCase.info, testCase.resourceNames, testCase.namespace)

		expectedGroupBy := testCase.expectedGroupBy
		if expectedGroupBy == "" {
			expectedGroupBy = registry.namer.labelPrefix + testCase.info.GroupResource.Resource
		}
		assert.Equal(expectedGroupBy, groupBy, "%s: metric %v should have produced the correct groupBy clause", testCase.title, testCase.info)
	}

	allMetrics := registry.ListAllMetrics()
	expectedMetrics := []provider.MetricInfo{
		{schema.GroupResource{Resource: "pods"}, true, "actually_gauge"},
		{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
		{schema.GroupResource{Resource: "pods"}, true, "some_count"},
		{schema.GroupResource{Resource: "pods"}, true, "some_time"},
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

// metricInfoSorter is a sort.Interface for sorting provider.MetricInfos
type metricInfoSorter []provider.MetricInfo

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
