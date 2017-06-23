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

	"k8s.io/client-go/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"

	// install extensions so that our RESTMapper knows about it
	_ "k8s.io/client-go/pkg/apis/extensions/install"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

func setupMetricNamer(t *testing.T) *metricNamer {
	return &metricNamer{
		overrides: map[string]seriesSpec{
			"container_actually_gauge_seconds_total": seriesSpec{
				metricName: "actually_gauge",
				kind: GaugeSeries,
			},
		},
		mapper: api.Registry.RESTMapper(),
	}
}

func TestMetricNamerContainerSeries(t *testing.T) {
	testCases := []struct{
		input prom.Series
		outputMetricName string
		outputInfo seriesInfo
	}{
		{
			input: prom.Series{
				Name: "container_actually_gauge_seconds_total",
				Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "actually_gauge",
			outputInfo: seriesInfo{
				baseSeries: prom.Series{Name: "container_actually_gauge_seconds_total"},
				kind: GaugeSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name: "container_some_usage",
				Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_usage",
			outputInfo: seriesInfo{
				baseSeries: prom.Series{Name: "container_some_usage"},
				kind: GaugeSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name: "container_some_count_total",
				Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_count",
			outputInfo: seriesInfo{
				baseSeries: prom.Series{Name: "container_some_count_total"},
				kind: CounterSeries,
				isContainer: true,
			},
		},
		{
			input: prom.Series{
				Name: "container_some_time_seconds_total",
				Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
			},
			outputMetricName: "some_time",
			outputInfo: seriesInfo{
				baseSeries: prom.Series{Name: "container_some_time_seconds_total"},
				kind: SecondsCounterSeries,
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
			Metric: test.outputMetricName,
			GroupResource: schema.GroupResource{Resource: "pods"},
			Namespaced: true,
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
			Name: "container_actually_gauge_seconds_total",
			Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name: "container_some_usage",
			Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name: "container_some_count_total",
			Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		{
			Name: "container_some_time_seconds_total",
			Labels: map[string]string{"pod_name": "somepod", "namespace": "somens", "container_name": "somecont"},
		},
		// namespaced series
		// a series that should turn into multiple metrics
		{
			Name: "ingress_hits_total",
			Labels: map[string]string{"ingress": "someingress", "service": "somesvc", "pod": "backend1", "namespace": "somens"},
		},
		{
			Name: "ingress_hits_total",
			Labels: map[string]string{"ingress": "someingress", "service": "somesvc", "pod": "backend2", "namespace": "somens"},
		},
		{
			Name: "service_proxy_packets",
			Labels: map[string]string{"service": "somesvc", "namespace": "somens"},
		},
		{
			Name: "work_queue_wait_seconds_total",
			Labels: map[string]string{"deployment": "somedep", "namespace": "somens"},
		},
		// non-namespaced series
		{
			Name: "node_gigawatts",
			Labels: map[string]string{"node": "somenode"},
		},
		{
			Name: "volume_claims_total",
			Labels: map[string]string{"persistentvolume": "somepv"},
		},
		{
			Name: "node_fan_seconds_total",
			Labels: map[string]string{"node": "somenode"},
		},
		// unrelated series
		{
			Name: "admin_coffee_liters_total",
			Labels: map[string]string{"admin": "some-admin"},
		},
		{
			Name: "admin_unread_emails",
			Labels: map[string]string{"admin": "some-admin"},
		},
		{
			Name: "admin_reddit_seconds_total",
			Labels: map[string]string{"admin": "some-admin"},
		},
	}

	// set up the registry
	require.NoError(registry.SetSeries(inputSeries))

	// make sure each metric got registered and can form queries
	testCases := []struct{
		title string
		info provider.MetricInfo
		namespace string
		resourceNames []string

		expectedKind SeriesType
		expectedQuery string
	}{
		// container metrics
		{
			title: "container metrics overrides / single resource name",
			info: provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "actually_gauge"},
			namespace: "somens",
			resourceNames: []string{"somepod"},

			expectedKind: GaugeSeries,
			expectedQuery: "container_actually_gauge_seconds_total{pod_name=\"somepod\",container_name!=\"POD\",namespace=\"somens\"}",
		},
		{
			title: "container metrics gauge / multiple resource names",
			info: provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
			namespace: "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind: GaugeSeries,
			expectedQuery: "container_some_usage{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
		},
		{
			title: "container metrics counter",
			info: provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
			namespace: "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind: CounterSeries,
			expectedQuery: "container_some_count_total{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
		},
		{
			title: "container metrics seconds counter",
			info: provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
			namespace: "somens",
			resourceNames: []string{"somepod1", "somepod2"},

			expectedKind: SecondsCounterSeries,
			expectedQuery: "container_some_time_seconds_total{pod_name=~\"somepod1|somepod2\",container_name!=\"POD\",namespace=\"somens\"}",
		},
		// namespaced metrics
		{
			title: "namespaced metrics counter / multidimensional (service)",
			info: provider.MetricInfo{schema.GroupResource{Resource: "service"}, true, "ingress_hits"},
			namespace: "somens",
			resourceNames: []string{"somesvc"},

			expectedKind: CounterSeries,
			expectedQuery: "ingress_hits_total{service=\"somesvc\",namespace=\"somens\"}",
		},
		{
			title: "namespaced metrics counter / multidimensional (ingress)",
			info: provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingress"}, true, "ingress_hits"},
			namespace: "somens",
			resourceNames: []string{"someingress"},

			expectedKind: CounterSeries,
			expectedQuery: "ingress_hits_total{ingress=\"someingress\",namespace=\"somens\"}",
		},
		{
			title: "namespaced metrics counter / multidimensional (pod)",
			info: provider.MetricInfo{schema.GroupResource{Resource: "pod"}, true, "ingress_hits"},
			namespace: "somens",
			resourceNames: []string{"somepod"},

			expectedKind: CounterSeries,
			expectedQuery: "ingress_hits_total{pod=\"somepod\",namespace=\"somens\"}",
		},
		{
			title: "namespaced metrics gauge",
			info: provider.MetricInfo{schema.GroupResource{Resource: "service"}, true, "service_proxy_packets"},
			namespace: "somens",
			resourceNames: []string{"somesvc"},

			expectedKind: GaugeSeries,
			expectedQuery: "service_proxy_packets{service=\"somesvc\",namespace=\"somens\"}",
		},
		{
			title: "namespaced metrics seconds counter",
			info: provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployment"}, true, "work_queue_wait"},
			namespace: "somens",
			resourceNames: []string{"somedep"},

			expectedKind: SecondsCounterSeries,
			expectedQuery: "work_queue_wait_seconds_total{deployment=\"somedep\",namespace=\"somens\"}",
		},
		// non-namespaced series
		{
			title: "root scoped metrics gauge",
			info: provider.MetricInfo{schema.GroupResource{Resource: "node"}, false, "node_gigawatts"},
			resourceNames: []string{"somenode"},

			expectedKind: GaugeSeries,
			expectedQuery: "node_gigawatts{node=\"somenode\"}",
		},
		{
			title: "root scoped metrics counter",
			info: provider.MetricInfo{schema.GroupResource{Resource: "persistentvolume"}, false, "volume_claims"},
			resourceNames: []string{"somepv"},

			expectedKind: CounterSeries,
			expectedQuery: "volume_claims_total{persistentvolume=\"somepv\"}",
		},
		{
			title: "root scoped metrics seconds counter",
			info: provider.MetricInfo{schema.GroupResource{Resource: "node"}, false, "node_fan"},
			resourceNames: []string{"somenode"},

			expectedKind: SecondsCounterSeries,
			expectedQuery: "node_fan_seconds_total{node=\"somenode\"}",
		},
	}

	for _, testCase := range testCases {
		outputKind, outputQuery, found := registry.QueryForMetric(testCase.info, testCase.namespace, testCase.resourceNames...)
		if !assert.True(found, "%s: metric %v should available", testCase.title, testCase.info) {
			continue
		}

		assert.Equal(testCase.expectedKind, outputKind, "%s: metric %v should have had the right series type", testCase.title, testCase.info)
		assert.Equal(prom.Selector(testCase.expectedQuery), outputQuery, "%s: metric %v should have produced the correct query for %v in namespace %s", testCase.title, testCase.info, testCase.resourceNames, testCase.namespace)
	}

	allMetrics := registry.ListAllMetrics()
	expectedMetrics := []provider.MetricInfo{
		provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "actually_gauge"},
		provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
		provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
		provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
		provider.MetricInfo{schema.GroupResource{Resource: "services"}, true, "ingress_hits"},
		provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingresses"}, true, "ingress_hits"},
		provider.MetricInfo{schema.GroupResource{Resource: "pods"}, true, "ingress_hits"},
		provider.MetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "ingress_hits"},
		provider.MetricInfo{schema.GroupResource{Resource: "services"}, true, "service_proxy_packets"},
		provider.MetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "service_proxy_packets"},
		provider.MetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployments"}, true, "work_queue_wait"},
		provider.MetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "work_queue_wait"},
		provider.MetricInfo{schema.GroupResource{Resource: "nodes"}, false, "node_gigawatts"},
		provider.MetricInfo{schema.GroupResource{Resource: "persistentvolumes"}, false, "volume_claims"},
		provider.MetricInfo{schema.GroupResource{Resource: "nodes"}, false, "node_fan"},
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
