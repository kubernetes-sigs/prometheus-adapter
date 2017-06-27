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
	"strings"
	"sync"

	"github.com/directxman12/custom-metrics-boilerplate/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/golang/glog"
	pmodel "github.com/prometheus/common/model"
)

// NB: container metrics sourced from cAdvisor don't consistently follow naming conventions,
// so we need to whitelist them and handle them on a case-by-case basis.  Metrics ending in `_total`
// *should* be counters, but may actually be guages in this case.

// SeriesType represents the kind of series backing a metric.
type SeriesType int

const (
	CounterSeries SeriesType = iota
	SecondsCounterSeries
	GaugeSeries
)

// SeriesRegistry provides conversions between Prometheus series and MetricInfo
type SeriesRegistry interface {
	// SetSeries replaces the known series in this registry
	SetSeries(series []prom.Series) error
	// ListAllMetrics lists all metrics known to this registry
	ListAllMetrics() []provider.MetricInfo
	// SeriesForMetric looks up the minimum required series information to make a query for the given metric
	// against the given resource (namespace may be empty for non-namespaced resources)
	QueryForMetric(info provider.MetricInfo, namespace string, resourceNames ...string) (kind SeriesType, query prom.Selector, groupBy string, found bool)
	// MatchValuesToNames matches result values to resource names for the given metric and value set
	MatchValuesToNames(metricInfo provider.MetricInfo, values pmodel.Vector) (matchedValues map[string]pmodel.SampleValue, found bool)
}

type seriesInfo struct {
	// baseSeries represents the minimum information to access a particular series
	baseSeries prom.Series
	// kind is the type of this series
	kind SeriesType
	// isContainer indicates if the series is a cAdvisor container_ metric, and thus needs special handling
	isContainer bool
}

// overridableSeriesRegistry is a basic SeriesRegistry
type basicSeriesRegistry struct {
	mu sync.RWMutex

	// info maps metric info to information about the corresponding series
	info map[provider.MetricInfo]seriesInfo
	// metrics is the list of all known metrics
	metrics []provider.MetricInfo

	// namer is the metricNamer responsible for converting series to metric names and information
	namer metricNamer
}

func (r *basicSeriesRegistry) SetSeries(newSeries []prom.Series) error {
	newInfo := make(map[provider.MetricInfo]seriesInfo)
	for _, series := range newSeries {
		if strings.HasPrefix(series.Name, "container_") {
			r.namer.processContainerSeries(series, newInfo)
		} else if namespaceLabel, hasNamespaceLabel := series.Labels["namespace"]; hasNamespaceLabel && namespaceLabel != "" {
			// TODO: handle metrics describing a namespace
			if err := r.namer.processNamespacedSeries(series, newInfo); err != nil {
				// TODO: do we want to log this and continue, or abort?
				return err
			}
		} else {
			if err := r.namer.processRootScopedSeries(series, newInfo); err != nil {
				// TODO: do we want to log this and continue, or abort?
				return err
			}
		}
	}

	newMetrics := make([]provider.MetricInfo, 0, len(newInfo))
	for info := range newInfo {
		newMetrics = append(newMetrics, info)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.info = newInfo
	r.metrics = newMetrics

	return nil
}

func (r *basicSeriesRegistry) ListAllMetrics() []provider.MetricInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.metrics
}

func (r *basicSeriesRegistry) QueryForMetric(metricInfo provider.MetricInfo, namespace string, resourceNames ...string) (kind SeriesType, query prom.Selector, groupBy string, found bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(resourceNames) == 0 {
		// TODO: return error?  panic?
	}

	metricInfo, singularResource, err := r.namer.normalizeInfo(metricInfo)
	if err != nil {
		glog.Errorf("unable to normalize group resource while producing a query: %v", err)
		return 0, "", "", false
	}

	// TODO: support container metrics
	if info, found := r.info[metricInfo]; found {
		targetValue := resourceNames[0]
		matcher := prom.LabelEq
		if len(resourceNames) > 1 {
			targetValue = strings.Join(resourceNames, "|")
			matcher = prom.LabelMatches
		}

		var expressions []string
		if info.isContainer {
			expressions = []string{matcher("pod_name", targetValue), prom.LabelNeq("container_name", "POD")}
			groupBy = "pod_name"
		} else {
			// TODO: copy base series labels?
			expressions = []string{matcher(singularResource, targetValue)}
			groupBy = singularResource
		}

		if metricInfo.Namespaced {
			expressions = append(expressions, prom.LabelEq("namespace", namespace))
		}

		return info.kind, prom.MatchSeries(info.baseSeries.Name, expressions...), groupBy, true
	}

	glog.V(10).Infof("metric %v not registered", metricInfo)
	return 0, "", "", false
}

func (r *basicSeriesRegistry) MatchValuesToNames(metricInfo provider.MetricInfo, values pmodel.Vector) (matchedValues map[string]pmodel.SampleValue, found bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metricInfo, singularResource, err := r.namer.normalizeInfo(metricInfo)
	if err != nil {
		glog.Errorf("unable to normalize group resource while matching values to names: %v", err)
		return nil, false
	}

	if info, found := r.info[metricInfo]; found {
		res := make(map[string]pmodel.SampleValue, len(values))
		for _, val := range values {
			if val == nil {
				// skip empty values
				continue
			}

			labelName := pmodel.LabelName(singularResource)
			if info.isContainer {
				labelName = pmodel.LabelName("pod_name")
			}
			res[string(val.Metric[labelName])] = val.Value
		}

		return res, true
	}

	return nil, false
}

// metricNamer knows how to construct MetricInfo out of raw prometheus series descriptions.
type metricNamer struct {
	// overrides contains the list of container metrics whose naming we want to override.
	// This is used to properly convert certain cAdvisor container metrics.
	overrides map[string]seriesSpec

	mapper apimeta.RESTMapper
}

// seriesSpec specifies how to produce metric info for a particular prometheus series source
type seriesSpec struct {
	// metricName is the desired output API metric name
	metricName string
	// kind indicates whether or not this metric is cumulative,
	// and thus has to be calculated as a rate when returning it
	kind SeriesType
}

// normalizeInfo takes in some metricInfo an "normalizes" it to ensure a common GroupResource form.
func (r *metricNamer) normalizeInfo(metricInfo provider.MetricInfo) (provider.MetricInfo, string, error) {
	// NB: we need to "normalize" the metricInfo's GroupResource so we have a consistent pluralization, etc
	// TODO: move this to the boilerplate?
	normalizedGroupRes, err := r.mapper.ResourceFor(metricInfo.GroupResource.WithVersion(""))
	if err != nil {
		return provider.MetricInfo{}, "", err
	}
	metricInfo.GroupResource = normalizedGroupRes.GroupResource()

	singularResource, err := r.mapper.ResourceSingularizer(metricInfo.GroupResource.Resource)
	if err != nil {
		return provider.MetricInfo{}, "", err
	}

	return metricInfo, singularResource, nil
}

// processContainerSeries performs special work to extract metric definitions
// from cAdvisor-sourced container metrics, which don't particularly follow any useful conventions consistently.
func (n *metricNamer) processContainerSeries(series prom.Series, infos map[provider.MetricInfo]seriesInfo) {

	originalName := series.Name

	var name string
	metricKind := GaugeSeries
	if override, hasOverride := n.overrides[series.Name]; hasOverride {
		name = override.metricName
		metricKind = override.kind
	} else {
		// chop of the "container_" prefix
		series.Name = series.Name[10:]
		name, metricKind = n.metricNameFromSeries(series)
	}

	info := provider.MetricInfo{
		// TODO: is the plural correct?
		GroupResource: schema.GroupResource{Resource: "pods"},
		Namespaced:    true,
		Metric:        name,
	}

	infos[info] = seriesInfo{
		kind:        metricKind,
		baseSeries:  prom.Series{Name: originalName},
		isContainer: true,
	}
}

// processNamespacedSeries adds the metric info for the given generic namespaced series to
// the map of metric info.
func (n *metricNamer) processNamespacedSeries(series prom.Series, infos map[provider.MetricInfo]seriesInfo) error {
	name, metricKind := n.metricNameFromSeries(series)
	resources, err := n.groupResourcesFromSeries(series)
	if err != nil {
		return fmt.Errorf("unable to process prometheus series %s: %v", series.Name, err)
	}

	// we add one metric for each resource that this could describe
	for _, resource := range resources {
		info := provider.MetricInfo{
			GroupResource: resource,
			Namespaced:    true,
			Metric:        name,
		}

		// metrics describing namespaces aren't considered to be namespaced
		if resource == (schema.GroupResource{Resource: "namespaces"}) {
			info.Namespaced = false
		}

		infos[info] = seriesInfo{
			kind:       metricKind,
			baseSeries: prom.Series{Name: series.Name},
		}
	}

	return nil
}

// processesRootScopedSeries adds the metric info for the given generic namespaced series to
// the map of metric info.
func (n *metricNamer) processRootScopedSeries(series prom.Series, infos map[provider.MetricInfo]seriesInfo) error {
	name, metricKind := n.metricNameFromSeries(series)
	resources, err := n.groupResourcesFromSeries(series)
	if err != nil {
		return fmt.Errorf("unable to process prometheus series %s: %v", series.Name, err)
	}

	// we add one metric for each resource that this could describe
	for _, resource := range resources {
		info := provider.MetricInfo{
			GroupResource: resource,
			Namespaced:    false,
			Metric:        name,
		}

		infos[info] = seriesInfo{
			kind:       metricKind,
			baseSeries: prom.Series{Name: series.Name},
		}
	}

	return nil
}

// groupResourceFromSeries collects the possible group-resources that this series could describe by
// going through each label, checking to see if it corresponds to a known resource.  For instance,
// a series `ingress_http_hits_total{pod="foo",service="bar",ingress="baz",namespace="ns"}`
// would return three GroupResources: "pods", "services", and "ingresses".
// Returned MetricInfo is equilavent to the "normalized" info produced by normalizeInfo.
func (n *metricNamer) groupResourcesFromSeries(series prom.Series) ([]schema.GroupResource, error) {
	// TODO: do we need to cache this, or is ResourceFor good enough?
	var res []schema.GroupResource
	for label := range series.Labels {
		// TODO: figure out a way to let people specify a fully-qualified name in label-form
		// TODO: will this work when missing a group?
		gvr, err := n.mapper.ResourceFor(schema.GroupVersionResource{Resource: string(label)})
		if err != nil {
			if apimeta.IsNoMatchError(err) {
				continue
			}
			return nil, err
		}
		res = append(res, gvr.GroupResource())
	}

	return res, nil
}

// metricNameFromSeries extracts a metric name from a series name, and indicates
// whether or not that series was a counter.  It also has special logic to deal with time-based
// counters, which general get converted to milli-unit rate metrics.
func (n *metricNamer) metricNameFromSeries(series prom.Series) (name string, kind SeriesType) {
	kind = GaugeSeries
	name = series.Name
	if strings.HasSuffix(name, "_total") {
		kind = CounterSeries
		name = name[:len(name)-6]

		if strings.HasSuffix(name, "_seconds") {
			kind = SecondsCounterSeries
			name = name[:len(name)-8]
		}
	}

	return
}
