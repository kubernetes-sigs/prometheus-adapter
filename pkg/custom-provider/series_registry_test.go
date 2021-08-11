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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	pmodel "github.com/prometheus/common/model"

	coreapi "k8s.io/api/core/v1"
	extapi "k8s.io/api/extensions/v1beta1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	config "sigs.k8s.io/prometheus-adapter/cmd/config-gen/utils"
	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"
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

func setupMetricNamer() []naming.MetricNamer {
	cfg := config.DefaultConfig(1*time.Minute, "kube_")
	namers, err := naming.NamersFromConfig(cfg.Rules, restMapper())
	Expect(err).NotTo(HaveOccurred())
	return namers
}

var seriesRegistryTestSeries = [][]prom.Series{
	// container series
	{
		{
			Name:   "container_some_time_seconds_total",
			Labels: pmodel.LabelSet{"pod": "somepod", "namespace": "somens", "container": "somecont"},
		},
	},
	{
		{
			Name:   "container_some_count_total",
			Labels: pmodel.LabelSet{"pod": "somepod", "namespace": "somens", "container": "somecont"},
		},
	},
	{
		{
			Name:   "container_some_usage",
			Labels: pmodel.LabelSet{"pod": "somepod", "namespace": "somens", "container": "somecont"},
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

type regTestCase struct {
	title          string
	info           provider.CustomMetricInfo
	namespace      string
	resourceNames  []string
	metricSelector labels.Selector

	expectedQuery string
}

func mustNewLabelRequirement(key string, op selection.Operator, vals []string) *labels.Requirement {
	req, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(err)
	}
	return req
}

var _ = Describe("Series Registry", func() {
	var (
		registry *basicSeriesRegistry
	)

	BeforeEach(func() {
		namers := setupMetricNamer()
		registry = &basicSeriesRegistry{
			mapper: restMapper(),
		}
		Expect(registry.SetSeries(seriesRegistryTestSeries, namers)).To(Succeed())
	})

	Context("with the default configuration rules", func() {
		// make sure each metric got registered and can form queries
		testCases := []regTestCase{
			// container metrics
			{
				title:          "container metrics gauge / multiple resource names",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
				namespace:      "somens",
				resourceNames:  []string{"somepod1", "somepod2"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(container_some_usage{namespace=\"somens\",pod=~\"somepod1|somepod2\",container!=\"POD\"}) by (pod)",
			},
			{
				title:          "container metrics counter",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
				namespace:      "somens",
				resourceNames:  []string{"somepod1", "somepod2"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(container_some_count_total{namespace=\"somens\",pod=~\"somepod1|somepod2\",container!=\"POD\"}[1m])) by (pod)",
			},
			{
				title:          "container metrics seconds counter",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
				namespace:      "somens",
				resourceNames:  []string{"somepod1", "somepod2"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(container_some_time_seconds_total{namespace=\"somens\",pod=~\"somepod1|somepod2\",container!=\"POD\"}[1m])) by (pod)",
			},
			// namespaced metrics
			{
				title:          "namespaced metrics counter / multidimensional (service)",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "service"}, true, "ingress_hits"},
				namespace:      "somens",
				resourceNames:  []string{"somesvc"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_service=\"somesvc\"}[1m])) by (kube_service)",
			},
			{
				title:         "namespaced metrics counter /  multidimensional (service) / selection using labels",
				info:          provider.CustomMetricInfo{schema.GroupResource{Resource: "service"}, true, "ingress_hits"},
				namespace:     "somens",
				resourceNames: []string{"somesvc"},
				metricSelector: labels.NewSelector().Add(
					*mustNewLabelRequirement("param1", selection.Equals, []string{"value1"}),
				),
				expectedQuery: "sum(rate(ingress_hits_total{param1=\"value1\",kube_namespace=\"somens\",kube_service=\"somesvc\"}[1m])) by (kube_service)",
			},
			{
				title:          "namespaced metrics counter / multidimensional (ingress)",
				info:           provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingress"}, true, "ingress_hits"},
				namespace:      "somens",
				resourceNames:  []string{"someingress"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_ingress=\"someingress\"}[1m])) by (kube_ingress)",
			},
			{
				title:          "namespaced metrics counter / multidimensional (pod)",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "pod"}, true, "ingress_hits"},
				namespace:      "somens",
				resourceNames:  []string{"somepod"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(ingress_hits_total{kube_namespace=\"somens\",kube_pod=\"somepod\"}[1m])) by (kube_pod)",
			},
			{
				title:          "namespaced metrics gauge",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "service"}, true, "service_proxy_packets"},
				namespace:      "somens",
				resourceNames:  []string{"somesvc"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(service_proxy_packets{kube_namespace=\"somens\",kube_service=\"somesvc\"}) by (kube_service)",
			},
			{
				title:          "namespaced metrics seconds counter",
				info:           provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployment"}, true, "work_queue_wait"},
				namespace:      "somens",
				resourceNames:  []string{"somedep"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(work_queue_wait_seconds_total{kube_namespace=\"somens\",kube_deployment=\"somedep\"}[1m])) by (kube_deployment)",
			},
			// non-namespaced series
			{
				title:          "root scoped metrics gauge",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "node"}, false, "node_gigawatts"},
				resourceNames:  []string{"somenode"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(node_gigawatts{kube_node=\"somenode\"}) by (kube_node)",
			},
			{
				title:          "root scoped metrics counter",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "persistentvolume"}, false, "volume_claims"},
				resourceNames:  []string{"somepv"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(volume_claims_total{kube_persistentvolume=\"somepv\"}[1m])) by (kube_persistentvolume)",
			},
			{
				title:          "root scoped metrics seconds counter",
				info:           provider.CustomMetricInfo{schema.GroupResource{Resource: "node"}, false, "node_fan"},
				resourceNames:  []string{"somenode"},
				metricSelector: labels.Everything(),

				expectedQuery: "sum(rate(node_fan_seconds_total{kube_node=\"somenode\"}[1m])) by (kube_node)",
			},
		}

		for _, tc := range testCases {
			tc := tc // copy to avoid iteration variable issues
			It(fmt.Sprintf("should build a query for %s", tc.title), func() {
				By(fmt.Sprintf("composing the query for the %s metric on %v in namespace %s", tc.info, tc.resourceNames, tc.namespace))
				outputQuery, found := registry.QueryForMetric(tc.info, tc.namespace, tc.metricSelector, tc.resourceNames...)
				Expect(found).To(BeTrue(), "metric %s should be available", tc.info)

				By("verifying that the query is as expected")
				Expect(outputQuery).To(Equal(prom.Selector(tc.expectedQuery)))
			})
		}

		It("should list all metrics", func() {
			Expect(registry.ListAllMetrics()).To(ConsistOf(
				provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_count"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "some_count"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_time"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "some_time"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "some_usage"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "some_usage"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "services"}, true, "ingress_hits"},
				provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "ingresses"}, true, "ingress_hits"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "pods"}, true, "ingress_hits"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "ingress_hits"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "services"}, true, "service_proxy_packets"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "service_proxy_packets"},
				provider.CustomMetricInfo{schema.GroupResource{Group: "extensions", Resource: "deployments"}, true, "work_queue_wait"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "namespaces"}, false, "work_queue_wait"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "nodes"}, false, "node_gigawatts"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "persistentvolumes"}, false, "volume_claims"},
				provider.CustomMetricInfo{schema.GroupResource{Resource: "nodes"}, false, "node_fan"},
			))
		})
	})
})
