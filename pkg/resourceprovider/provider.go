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
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	metrics "k8s.io/metrics/pkg/apis/metrics"

	"sigs.k8s.io/metrics-server/pkg/api"

	"sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/config"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"

	pmodel "github.com/prometheus/common/model"
)

var (
	nodeResource = schema.GroupResource{Resource: "nodes"}
	nsResource   = schema.GroupResource{Resource: "ns"}
	podResource  = schema.GroupResource{Resource: "pods"}
)

// TODO(directxman12): consider support for nanocore values -- adjust scale if less than 1 millicore, or greater than max int64

// newResourceQuery instantiates query information from the give configuration rule for querying
// resource metrics for some resource.
func newResourceQuery(cfg config.ResourceRule, mapper apimeta.RESTMapper) (resourceQuery, error) {
	converter, err := naming.NewResourceConverter(cfg.Resources.Template, cfg.Resources.Overrides, mapper)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct label-resource converter: %v", err)
	}

	contQuery, err := naming.NewMetricsQuery(cfg.ContainerQuery, converter)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct container metrics query: %v", err)
	}
	nodeQuery, err := naming.NewMetricsQuery(cfg.NodeQuery, converter)
	if err != nil {
		return resourceQuery{}, fmt.Errorf("unable to construct node metrics query: %v", err)
	}

	return resourceQuery{
		converter:      converter,
		contQuery:      contQuery,
		nodeQuery:      nodeQuery,
		containerLabel: cfg.ContainerLabel,
	}, nil

}

// resourceQuery represents query information for querying resource metrics for some resource,
// like CPU or memory.
type resourceQuery struct {
	converter      naming.ResourceConverter
	contQuery      naming.MetricsQuery
	nodeQuery      naming.MetricsQuery
	containerLabel string
}

// NewProvider constructs a new MetricsProvider to provide resource metrics from Prometheus using the given rules.
func NewProvider(prom client.Client, mapper apimeta.RESTMapper, cfg *config.ResourceRules) (api.MetricsGetter, error) {
	cpuQuery, err := newResourceQuery(cfg.CPU, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct querier for CPU metrics: %v", err)
	}
	memQuery, err := newResourceQuery(cfg.Memory, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct querier for memory metrics: %v", err)
	}

	return &resourceProvider{
		prom:   prom,
		cpu:    cpuQuery,
		mem:    memQuery,
		window: time.Duration(cfg.Window),
	}, nil
}

// resourceProvider is a MetricsProvider that contacts Prometheus to provide
// the resource metrics.
type resourceProvider struct {
	prom client.Client

	cpu, mem resourceQuery

	window time.Duration
}

// nsQueryResults holds the results of one set
// of queries necessary to construct a resource metrics
// API response for a single namespace.
type nsQueryResults struct {
	namespace string
	cpu, mem  queryResults
	err       error
}

// GetPodMetrics implements the api.MetricsProvider interface.
func (p *resourceProvider) GetPodMetrics(pods ...*metav1.PartialObjectMetadata) ([]metrics.PodMetrics, error) {
	resMetrics := make([]metrics.PodMetrics, 0, len(pods))

	if len(pods) == 0 {
		return resMetrics, nil
	}

	// TODO(directxman12): figure out how well this scales if we go to list 1000+ pods
	// (and consider adding timeouts)

	// group pods by namespace (we could be listing for all pods in the cluster)
	podsByNs := make(map[string][]string, len(pods))
	for _, pod := range pods {
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod.Name)
	}

	// actually fetch the results for each namespace
	now := pmodel.Now()
	resChan := make(chan nsQueryResults, len(podsByNs))
	var wg sync.WaitGroup
	wg.Add(len(podsByNs))

	for ns, podNames := range podsByNs {
		go func(ns string, podNames []string) {
			defer wg.Done()
			resChan <- p.queryBoth(now, podResource, ns, podNames...)
		}(ns, podNames)
	}

	wg.Wait()
	close(resChan)

	// index those results in a map for easy lookup
	resultsByNs := make(map[string]nsQueryResults, len(podsByNs))
	for result := range resChan {
		if result.err != nil {
			klog.Errorf("unable to fetch metrics for pods in namespace %q, skipping: %v", result.namespace, result.err)
			continue
		}
		resultsByNs[result.namespace] = result
	}

	// convert the unorganized per-container results into results grouped
	// together by namespace, pod, and container
	for _, pod := range pods {
		podMetric := p.assignForPod(pod, resultsByNs)
		if podMetric != nil {
			resMetrics = append(resMetrics, *podMetric)
		}
	}

	return resMetrics, nil
}

// assignForPod takes the resource metrics for all containers in the given pod
// from resultsByNs, and places them in MetricsProvider response format in resMetrics,
// also recording the earliest time in resTime.  It will return without operating if
// any data is missing.
func (p *resourceProvider) assignForPod(pod *metav1.PartialObjectMetadata, resultsByNs map[string]nsQueryResults) *metrics.PodMetrics {
	// check to make sure everything is present
	nsRes, nsResPresent := resultsByNs[pod.Namespace]
	if !nsResPresent {
		klog.Errorf("unable to fetch metrics for pods in namespace %q, skipping pod %s", pod.Namespace, pod.String())
		return nil
	}
	cpuRes, hasResult := nsRes.cpu[pod.Name]
	if !hasResult {
		klog.Errorf("unable to fetch CPU metrics for pod %s, skipping", pod.String())
		return nil
	}
	memRes, hasResult := nsRes.mem[pod.Name]
	if !hasResult {
		klog.Errorf("unable to fetch memory metrics for pod %s, skipping", pod.String())
		return nil
	}

	containerMetrics := make(map[string]metrics.ContainerMetrics)
	earliestTs := pmodel.Latest

	// organize all the CPU results
	for _, cpu := range cpuRes {
		containerName := string(cpu.Metric[pmodel.LabelName(p.cpu.containerLabel)])
		if _, present := containerMetrics[containerName]; !present {
			containerMetrics[containerName] = metrics.ContainerMetrics{
				Name:  containerName,
				Usage: corev1.ResourceList{},
			}
		}
		containerMetrics[containerName].Usage[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(cpu.Value*1000.0), resource.DecimalSI)
		if cpu.Timestamp.Before(earliestTs) {
			earliestTs = cpu.Timestamp
		}
	}

	// organize the memory results
	for _, mem := range memRes {
		containerName := string(mem.Metric[pmodel.LabelName(p.mem.containerLabel)])
		if _, present := containerMetrics[containerName]; !present {
			containerMetrics[containerName] = metrics.ContainerMetrics{
				Name:  containerName,
				Usage: corev1.ResourceList{},
			}
		}
		containerMetrics[containerName].Usage[corev1.ResourceMemory] = *resource.NewMilliQuantity(int64(mem.Value*1000.0), resource.BinarySI)
		if mem.Timestamp.Before(earliestTs) {
			earliestTs = mem.Timestamp
		}
	}

	// check for any containers that have either memory usage or CPU usage, but not both
	for _, containerMetric := range containerMetrics {
		_, hasMemory := containerMetric.Usage[corev1.ResourceMemory]
		_, hasCPU := containerMetric.Usage[corev1.ResourceCPU]
		if hasMemory && !hasCPU {
			containerMetric.Usage[corev1.ResourceCPU] = *resource.NewMilliQuantity(int64(0), resource.BinarySI)
		} else if hasCPU && !hasMemory {
			containerMetric.Usage[corev1.ResourceMemory] = *resource.NewMilliQuantity(int64(0), resource.BinarySI)
		}
	}

	podMetric := &metrics.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name:              pod.Name,
			Namespace:         pod.Namespace,
			Labels:            pod.Labels,
			CreationTimestamp: metav1.Now(),
		},
		// store the time in the final format
		Timestamp: metav1.NewTime(earliestTs.Time()),
		Window:    metav1.Duration{Duration: p.window},
	}

	// store the container metrics in the final format
	podMetric.Containers = make([]metrics.ContainerMetrics, 0, len(containerMetrics))
	for _, containerMetric := range containerMetrics {
		podMetric.Containers = append(podMetric.Containers, containerMetric)
	}

	return podMetric
}

// GetNodeMetrics implements the api.MetricsProvider interface.
func (p *resourceProvider) GetNodeMetrics(nodes ...*corev1.Node) ([]metrics.NodeMetrics, error) {
	resMetrics := make([]metrics.NodeMetrics, 0, len(nodes))

	if len(nodes) == 0 {
		return resMetrics, nil
	}

	now := pmodel.Now()
	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	// run the actual query
	qRes := p.queryBoth(now, nodeResource, "", nodeNames...)
	if qRes.err != nil {
		klog.Errorf("failed querying node metrics: %v", qRes.err)
		return resMetrics, nil
	}

	// organize the results
	for i, nodeName := range nodeNames {
		// skip if any data is missing
		rawCPUs, gotResult := qRes.cpu[nodeName]
		if !gotResult {
			klog.V(1).Infof("missing CPU for node %q, skipping", nodeName)
			continue
		}
		rawMems, gotResult := qRes.mem[nodeName]
		if !gotResult {
			klog.V(1).Infof("missing memory for node %q, skipping", nodeName)
			continue
		}

		rawMem := rawMems[0]
		rawCPU := rawCPUs[0]

		// use the earliest timestamp available (in order to be conservative
		// when determining if metrics are tainted by startup)
		ts := rawCPU.Timestamp.Time()
		if ts.After(rawMem.Timestamp.Time()) {
			ts = rawMem.Timestamp.Time()
		}

		// store the results
		resMetrics = append(resMetrics, metrics.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:              nodes[i].Name,
				Labels:            nodes[i].Labels,
				CreationTimestamp: metav1.Now(),
			},
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(int64(rawCPU.Value*1000.0), resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewMilliQuantity(int64(rawMem.Value*1000.0), resource.BinarySI),
			},
			Timestamp: metav1.NewTime(ts),
			Window:    metav1.Duration{Duration: p.window},
		})
	}

	return resMetrics, nil
}

// queryBoth queries for both CPU and memory metrics on the given
// Kubernetes API resource (pods or nodes), and errors out if
// either query fails.
func (p *resourceProvider) queryBoth(now pmodel.Time, resource schema.GroupResource, namespace string, names ...string) nsQueryResults {
	var cpuRes, memRes queryResults
	var cpuErr, memErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cpuRes, cpuErr = p.runQuery(now, p.cpu, resource, namespace, names...)
	}()
	go func() {
		defer wg.Done()
		memRes, memErr = p.runQuery(now, p.mem, resource, namespace, names...)
	}()
	wg.Wait()

	if cpuErr != nil {
		return nsQueryResults{
			namespace: namespace,
			err:       fmt.Errorf("unable to fetch node CPU metrics: %v", cpuErr),
		}
	}
	if memErr != nil {
		return nsQueryResults{
			namespace: namespace,
			err:       fmt.Errorf("unable to fetch node memory metrics: %v", memErr),
		}
	}

	return nsQueryResults{
		namespace: namespace,
		cpu:       cpuRes,
		mem:       memRes,
	}
}

// queryResults maps an object name to all the results matching that object
type queryResults map[string][]*pmodel.Sample

// runQuery actually queries Prometheus for the metric represented by the given query information, on
// the given Kubernetes API resource (pods or nodes).
func (p *resourceProvider) runQuery(now pmodel.Time, queryInfo resourceQuery, resource schema.GroupResource, namespace string, names ...string) (queryResults, error) {
	var query client.Selector
	var err error

	// build the query, which needs the special "container" group by if this is for pod metrics
	if resource == nodeResource {
		query, err = queryInfo.nodeQuery.Build("", resource, namespace, nil, labels.Everything(), names...)
	} else {
		extraGroupBy := []string{queryInfo.containerLabel}
		query, err = queryInfo.contQuery.Build("", resource, namespace, extraGroupBy, labels.Everything(), names...)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to construct query: %v", err)
	}

	// run the query
	rawRes, err := p.prom.Query(context.Background(), now, query)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %v", err)
	}

	if rawRes.Type != pmodel.ValVector || rawRes.Vector == nil {
		return nil, fmt.Errorf("invalid or empty value of non-vector type (%s) returned", rawRes.Type)
	}

	// check the appropriate label for the resource in question
	resourceLbl, err := queryInfo.converter.LabelForResource(resource)
	if err != nil {
		return nil, fmt.Errorf("unable to find label for resource %s: %v", resource.String(), err)
	}

	// associate the results back to each given pod or node
	res := make(queryResults, len(*rawRes.Vector))
	for _, sample := range *rawRes.Vector {
		// skip empty samples
		if sample == nil {
			continue
		}
		// replace NaN and negative values by zero
		if math.IsNaN(float64(sample.Value)) || sample.Value < 0 {
			sample.Value = 0
		}
		resKey := string(sample.Metric[resourceLbl])
		res[resKey] = append(res[resKey], sample)
	}

	return res, nil
}
