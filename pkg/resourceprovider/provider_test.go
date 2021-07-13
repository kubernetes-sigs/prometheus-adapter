/*
Copyright 2018 The Kubernetes Authors.

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

package resourceprovider

import (
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/metrics"

	"sigs.k8s.io/metrics-server/pkg/api"

	config "sigs.k8s.io/prometheus-adapter/cmd/config-gen/utils"
	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	fakeprom "sigs.k8s.io/prometheus-adapter/pkg/client/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pmodel "github.com/prometheus/common/model"
)

func restMapper() apimeta.RESTMapper {
	mapper := apimeta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})

	mapper.Add(corev1.SchemeGroupVersion.WithKind("Pod"), apimeta.RESTScopeNamespace)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Node"), apimeta.RESTScopeRoot)
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Namespace"), apimeta.RESTScopeRoot)

	return mapper
}

func buildPodSample(namespace, pod, container string, val float64, ts int64) *pmodel.Sample {
	return &pmodel.Sample{
		Metric: pmodel.Metric{
			"namespace": pmodel.LabelValue(namespace),
			"pod":       pmodel.LabelValue(pod),
			"container": pmodel.LabelValue(container),
		},
		Value:     pmodel.SampleValue(val),
		Timestamp: pmodel.Time(ts),
	}
}

func buildNodeSample(node string, val float64, ts int64) *pmodel.Sample {
	return &pmodel.Sample{
		Metric: pmodel.Metric{
			"instance": pmodel.LabelValue(node),
			"id":       "/",
		},
		Value:     pmodel.SampleValue(val),
		Timestamp: pmodel.Time(ts),
	}
}

func buildQueryRes(metric string, samples ...*pmodel.Sample) prom.QueryResult {
	for _, sample := range samples {
		sample.Metric[pmodel.MetricNameLabel] = pmodel.LabelValue(metric)
	}
	vec := pmodel.Vector(samples)
	return prom.QueryResult{
		Type:   pmodel.ValVector,
		Vector: &vec,
	}
}

func mustBuild(sel prom.Selector, err error) prom.Selector {
	Expect(err).NotTo(HaveOccurred())
	return sel
}

func buildResList(cpu, memory float64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(cpu*1000.0), resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewMilliQuantity(int64(memory*1000.0), resource.BinarySI),
	}
}

var _ = Describe("Resource Metrics Provider", func() {
	var (
		prov                   api.MetricsGetter
		fakeProm               *fakeprom.FakePrometheusClient
		cpuQueries, memQueries resourceQuery
	)

	BeforeEach(func() {
		By("setting up a fake prometheus client and provider")
		mapper := restMapper()

		cfg := config.DefaultConfig(1*time.Minute, "")

		var err error
		cpuQueries, err = newResourceQuery(cfg.ResourceRules.CPU, mapper)
		Expect(err).NotTo(HaveOccurred())
		memQueries, err = newResourceQuery(cfg.ResourceRules.Memory, mapper)
		Expect(err).NotTo(HaveOccurred())

		fakeProm = &fakeprom.FakePrometheusClient{}
		fakeProm.AcceptableInterval = pmodel.Interval{End: pmodel.Latest}

		prov, err = NewProvider(fakeProm, restMapper(), cfg.ResourceRules)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should be able to list metrics pods across different namespaces", func() {
		pods := []types.NamespacedName{
			{Namespace: "some-ns", Name: "pod1"},
			{Namespace: "some-ns", Name: "pod3"},
			{Namespace: "other-ns", Name: "pod27"},
		}
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod3")): buildQueryRes("container_cpu_usage_seconds_total",
				buildPodSample("some-ns", "pod1", "cont1", 1100.0, 10),
				buildPodSample("some-ns", "pod1", "cont2", 1110.0, 20),
				buildPodSample("some-ns", "pod3", "cont1", 1300.0, 10),
				buildPodSample("some-ns", "pod3", "cont2", 1310.0, 20),
			),
			mustBuild(cpuQueries.contQuery.Build("", podResource, "other-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod27")): buildQueryRes("container_cpu_usage_seconds_total",
				buildPodSample("other-ns", "pod27", "cont1", 2200.0, 270),
			),
			mustBuild(memQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod3")): buildQueryRes("container_memory_working_set_bytes",
				buildPodSample("some-ns", "pod1", "cont1", 3100.0, 11),
				buildPodSample("some-ns", "pod1", "cont2", 3110.0, 21),
				buildPodSample("some-ns", "pod3", "cont1", 3300.0, 11),
				buildPodSample("some-ns", "pod3", "cont2", 3310.0, 21),
			),
			mustBuild(memQueries.contQuery.Build("", podResource, "other-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod27")): buildQueryRes("container_memory_working_set_bytes",
				buildPodSample("other-ns", "pod27", "cont1", 4200.0, 271),
			),
		}

		By("querying for metrics for some pods")
		times, metricVals, err := prov.GetPodMetrics(pods...)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the reported times for each are the earliest times for each pod")
		Expect(times).To(Equal([]api.TimeInfo{
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(270).Time(), Window: 1 * time.Minute},
		}))

		By("verifying that the right metrics were fetched")
		Expect(metricVals).To(HaveLen(3))
		Expect(metricVals[0]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(1100.0, 3100.0)},
			metrics.ContainerMetrics{Name: "cont2", Usage: buildResList(1110.0, 3110.0)},
		))
		Expect(metricVals[1]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(1300.0, 3300.0)},
			metrics.ContainerMetrics{Name: "cont2", Usage: buildResList(1310.0, 3310.0)},
		))

		Expect(metricVals[2]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(2200.0, 4200.0)},
		))
	})

	It("should return nil metrics for missing pods, but still return partial results", func() {
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod-nonexistant")): buildQueryRes("container_cpu_usage_seconds_total",
				buildPodSample("some-ns", "pod1", "cont1", 1100.0, 10),
				buildPodSample("some-ns", "pod1", "cont2", 1110.0, 20),
			),
			mustBuild(memQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod-nonexistant")): buildQueryRes("container_memory_working_set_bytes",
				buildPodSample("some-ns", "pod1", "cont1", 3100.0, 11),
				buildPodSample("some-ns", "pod1", "cont2", 3110.0, 21),
			),
		}

		By("querying for metrics for some pods, one of which is missing")
		times, metricVals, err := prov.GetPodMetrics(
			types.NamespacedName{Namespace: "some-ns", Name: "pod1"},
			types.NamespacedName{Namespace: "some-ns", Name: "pod-nonexistant"},
		)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the missing pod had nil metrics")
		Expect(metricVals).To(HaveLen(2))
		Expect(metricVals[1]).To(BeNil())

		By("verifying that the rest of time metrics and times are correct")
		Expect(metricVals[0]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(1100.0, 3100.0)},
			metrics.ContainerMetrics{Name: "cont2", Usage: buildResList(1110.0, 3110.0)},
		))
		Expect(times).To(HaveLen(2))
		Expect(times[0]).To(Equal(api.TimeInfo{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute}))
	})

	It("should return metrics of value zero when pod metrics have NaN or negative values", func() {
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod3")): buildQueryRes("container_cpu_usage_seconds_total",
				buildPodSample("some-ns", "pod1", "cont1", -1100.0, 10),
				buildPodSample("some-ns", "pod1", "cont2", math.NaN(), 20),
				buildPodSample("some-ns", "pod3", "cont1", -1300.0, 10),
				buildPodSample("some-ns", "pod3", "cont2", 1310.0, 20),
			),
			mustBuild(memQueries.contQuery.Build("", podResource, "some-ns", []string{cpuQueries.containerLabel}, labels.Everything(), "pod1", "pod3")): buildQueryRes("container_memory_working_set_bytes",
				buildPodSample("some-ns", "pod1", "cont1", 3100.0, 11),
				buildPodSample("some-ns", "pod1", "cont2", -3110.0, 21),
				buildPodSample("some-ns", "pod3", "cont1", math.NaN(), 11),
				buildPodSample("some-ns", "pod3", "cont2", -3310.0, 21),
			),
		}

		By("querying for metrics for some pods")
		times, metricVals, err := prov.GetPodMetrics(
			types.NamespacedName{Namespace: "some-ns", Name: "pod1"},
			types.NamespacedName{Namespace: "some-ns", Name: "pod3"},
		)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the reported times for each are the earliest times for each pod")
		Expect(times).To(Equal([]api.TimeInfo{
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
		}))

		By("verifying that NaN and negative values were replaced by zero")
		Expect(metricVals).To(HaveLen(2))
		Expect(metricVals[0]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(0, 3100.0)},
			metrics.ContainerMetrics{Name: "cont2", Usage: buildResList(0, 0)},
		))
		Expect(metricVals[1]).To(ConsistOf(
			metrics.ContainerMetrics{Name: "cont1", Usage: buildResList(0, 0)},
			metrics.ContainerMetrics{Name: "cont2", Usage: buildResList(1310.0, 0)},
		))
	})

	It("should be able to list metrics for nodes", func() {
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2")): buildQueryRes("container_cpu_usage_seconds_total",
				buildNodeSample("node1", 1100.0, 10),
				buildNodeSample("node2", 1200.0, 14),
			),
			mustBuild(memQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2")): buildQueryRes("container_memory_working_set_bytes",
				buildNodeSample("node1", 2100.0, 11),
				buildNodeSample("node2", 2200.0, 12),
			),
		}
		By("querying for metrics for some nodes")
		times, metricVals, err := prov.GetNodeMetrics("node1", "node2")
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the reported times for each are the earliest times for each pod")
		Expect(times).To(Equal([]api.TimeInfo{
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(12).Time(), Window: 1 * time.Minute},
		}))

		By("verifying that the right metrics were fetched")
		Expect(metricVals).To(Equal([]corev1.ResourceList{
			buildResList(1100.0, 2100.0),
			buildResList(1200.0, 2200.0),
		}))
	})

	It("should return nil metrics for missing nodes, but still return partial results", func() {
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2", "node3")): buildQueryRes("container_cpu_usage_seconds_total",
				buildNodeSample("node1", 1100.0, 10),
				buildNodeSample("node2", 1200.0, 14),
			),
			mustBuild(memQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2", "node3")): buildQueryRes("container_memory_working_set_bytes",
				buildNodeSample("node1", 2100.0, 11),
				buildNodeSample("node2", 2200.0, 12),
			),
		}
		By("querying for metrics for some nodes, one of which is missing")
		times, metricVals, err := prov.GetNodeMetrics("node1", "node2", "node3")
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the missing pod had nil metrics")
		Expect(metricVals).To(HaveLen(3))
		Expect(metricVals[2]).To(BeNil())

		By("verifying that the rest of time metrics and times are correct")
		Expect(metricVals).To(Equal([]corev1.ResourceList{
			buildResList(1100.0, 2100.0),
			buildResList(1200.0, 2200.0),
			nil,
		}))
		Expect(times).To(Equal([]api.TimeInfo{
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(12).Time(), Window: 1 * time.Minute},
			{},
		}))
	})

	It("should return metrics of value zero when node metrics have NaN or negative values", func() {
		fakeProm.QueryResults = map[prom.Selector]prom.QueryResult{
			mustBuild(cpuQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2")): buildQueryRes("container_cpu_usage_seconds_total",
				buildNodeSample("node1", -1100.0, 10),
				buildNodeSample("node2", 1200.0, 14),
			),
			mustBuild(memQueries.nodeQuery.Build("", nodeResource, "", nil, labels.Everything(), "node1", "node2")): buildQueryRes("container_memory_working_set_bytes",
				buildNodeSample("node1", 2100.0, 11),
				buildNodeSample("node2", math.NaN(), 12),
			),
		}
		By("querying for metrics for some nodes")
		times, metricVals, err := prov.GetNodeMetrics("node1", "node2")
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the reported times for each are the earliest times for each pod")
		Expect(times).To(Equal([]api.TimeInfo{
			{Timestamp: pmodel.Time(10).Time(), Window: 1 * time.Minute},
			{Timestamp: pmodel.Time(12).Time(), Window: 1 * time.Minute},
		}))

		By("verifying that NaN and negative values were replaced by zero")
		Expect(metricVals).To(Equal([]corev1.ResourceList{
			buildResList(0, 2100.0),
			buildResList(1200.0, 0),
		}))
	})
})
