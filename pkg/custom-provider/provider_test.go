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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pmodel "github.com/prometheus/common/model"

	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedyn "k8s.io/client-go/dynamic/fake"

	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	config "sigs.k8s.io/prometheus-adapter/cmd/config-gen/utils"
	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	fakeprom "sigs.k8s.io/prometheus-adapter/pkg/client/fake"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"
)

const fakeProviderUpdateInterval = 2 * time.Second
const fakeProviderStartDuration = 2 * time.Second

func setupPrometheusProvider() (provider.CustomMetricsProvider, *fakeprom.FakePrometheusClient) {
	fakeProm := &fakeprom.FakePrometheusClient{}
	fakeKubeClient := &fakedyn.FakeDynamicClient{}

	cfg := config.DefaultConfig(1*time.Minute, "")
	namers, err := naming.NamersFromConfig(cfg.Rules, restMapper())
	Expect(err).NotTo(HaveOccurred())

	prov, _ := NewPrometheusProvider(restMapper(), fakeKubeClient, fakeProm, namers, fakeProviderUpdateInterval, fakeProviderStartDuration, false, "")

	containerSel := prom.MatchSeries("", prom.NameMatches("^container_.*"), prom.LabelNeq("container", "POD"), prom.LabelNeq("namespace", ""), prom.LabelNeq("pod", ""))
	namespacedSel := prom.MatchSeries("", prom.LabelNeq("namespace", ""), prom.NameNotMatches("^container_.*"))
	fakeProm.SeriesResults = map[prom.Selector][]prom.Series{
		containerSel: {
			{
				Name:   "container_some_usage",
				Labels: pmodel.LabelSet{"pod": "somepod", "namespace": "somens", "container": "somecont"},
			},
		},
		namespacedSel: {
			{
				Name:   "ingress_hits_total",
				Labels: pmodel.LabelSet{"ingress": "someingress", "service": "somesvc", "pod": "backend1", "namespace": "somens"},
			},
			{
				Name:   "ingress_hits_total",
				Labels: pmodel.LabelSet{"ingress": "someingress", "service": "somesvc", "pod": "backend2", "namespace": "somens"},
			},
			{
				Name:   "service_proxy_packets",
				Labels: pmodel.LabelSet{"service": "somesvc", "namespace": "somens"},
			},
			{
				Name:   "work_queue_wait_seconds_total",
				Labels: pmodel.LabelSet{"deployment": "somedep", "namespace": "somens"},
			},
		},
	}

	return prov, fakeProm
}

var _ = Describe("Custom Metrics Provider", func() {
	It("should be able to list all metrics", func() {
		By("setting up the provider")
		prov, fakeProm := setupPrometheusProvider()

		By("ensuring that no metrics are present before we start listing")
		Expect(prov.ListAllMetrics()).To(BeEmpty())

		By("setting the acceptable interval to now until the next update, with a bit of wiggle room")
		startTime := pmodel.Now().Add(-1*fakeProviderUpdateInterval - fakeProviderUpdateInterval/10)
		fakeProm.AcceptableInterval = pmodel.Interval{Start: startTime, End: 0}

		By("updating the list of available metrics")
		// don't call RunUntil to avoid timing issue
		lister := prov.(*prometheusProvider).SeriesRegistry.(*cachingMetricsLister)
		Expect(lister.updateMetrics()).To(Succeed())

		By("listing all metrics, and checking that they contain the expected results")
		Expect(prov.ListAllMetrics()).To(ConsistOf(
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "services"}, Namespaced: true, Metric: "ingress_hits"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Group: "extensions", Resource: "ingresses"}, Namespaced: true, Metric: "ingress_hits"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "pods"}, Namespaced: true, Metric: "ingress_hits"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "namespaces"}, Namespaced: false, Metric: "ingress_hits"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "services"}, Namespaced: true, Metric: "service_proxy_packets"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "namespaces"}, Namespaced: false, Metric: "service_proxy_packets"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Group: "extensions", Resource: "deployments"}, Namespaced: true, Metric: "work_queue_wait"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "namespaces"}, Namespaced: false, Metric: "work_queue_wait"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "namespaces"}, Namespaced: false, Metric: "some_usage"},
			provider.CustomMetricInfo{GroupResource: schema.GroupResource{Resource: "pods"}, Namespaced: true, Metric: "some_usage"},
		))
	})
})
