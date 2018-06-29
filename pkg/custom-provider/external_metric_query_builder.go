package provider

import (
	"fmt"
	s "strings"

	provider "github.com/directxman12/k8s-prometheus-adapter/pkg/custom-provider/metric-converter"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type ExternalMetricQueryBuilder interface {
	BuildPrometheusQuery(namespace string, metricName string, metricSelector labels.Selector, queryMetadata provider.QueryMetadata) string
}

type externalMetricQueryBuilder struct {
}

func NewExternalMetricQueryBuilder() ExternalMetricQueryBuilder {
	return &externalMetricQueryBuilder{}
}

func (p *externalMetricQueryBuilder) BuildPrometheusQuery(namespace string, metricName string, metricSelector labels.Selector, queryMetadata provider.QueryMetadata) string {
	//TODO: At least for my Prometheus install, the "namespace" label doesn't seem to be
	//directly applied to the time series. I'm using prometheus-operator. The grafana dashboards
	//seem to query for the pods in a namespace from kube_pod_info and then apply pod-specific
	//label filters. This might need some more thought. Disabling for now.
	// namespaceSelector := p.makeLabelFilter("namespace", "=", namespace)
	labelSelectors := p.convertSelectors(metricSelector)
	joinedLabels := s.Join(labelSelectors, ", ")

	//TODO: Both the aggregation method and window should probably be configurable.
	//I don't think we can make assumptions about the nature of someone's metrics.
	//I'm guessing this might be covered by the recently added advanced configuration
	//code, but I haven't yet had an opportunity to dig into that and understand it.
	//We'll leave this here for testing purposes for now.
	//As reasonable defaults, maybe:
	//rate(...) for counters
	//avg_over_time(...) for gauges
	//I'm guessing that SeriesRegistry might store the metric type, but I haven't looked yet.
	aggregation := queryMetadata.Aggregation
	window := queryMetadata.WindowInSeconds
	return fmt.Sprintf("%s(%s{%s}[%ss])", aggregation, metricName, joinedLabels, window)
}

func (p *externalMetricQueryBuilder) makeLabelFilter(labelName string, operator string, targetValue string) string {
	return fmt.Sprintf("%s%s\"%s\"", labelName, operator, targetValue)
}

func (p *externalMetricQueryBuilder) convertSelectors(metricSelector labels.Selector) []string {
	requirements, _ := metricSelector.Requirements()

	selectors := []string{}
	for i := 0; i < len(requirements); i++ {
		selector := p.convertRequirement(requirements[i])

		selectors = append(selectors, selector)
	}

	return selectors
}

func (p *externalMetricQueryBuilder) convertRequirement(requirement labels.Requirement) string {
	labelName := requirement.Key()
	values := requirement.Values().List()

	stringValues := values[0]

	valueCount := len(values)
	if valueCount > 1 {
		stringValues = s.Join(values, "|")
	}

	operator := p.selectOperator(requirement.Operator(), valueCount)

	return p.makeLabelFilter(labelName, operator, stringValues)
}

func (p *externalMetricQueryBuilder) selectOperator(operator selection.Operator, valueCount int) string {
	if valueCount > 1 {
		return p.selectRegexOperator(operator)
	}

	return p.selectSingleValueOperator(operator)
}

func (p *externalMetricQueryBuilder) selectRegexOperator(operator selection.Operator) string {
	switch operator {
	case selection.Equals:
	case selection.In:
		return "=~"
	case selection.NotIn:
	case selection.NotEquals:
		return "!~"
	}

	//TODO: Cover more cases, supply appropriate errors for any unhandled cases.
	return "="
}

func (p *externalMetricQueryBuilder) selectSingleValueOperator(operator selection.Operator) string {
	switch operator {
	case selection.Equals:
		return "="
	case selection.NotEquals:
		return "!="
	}

	//TODO: Cover more cases, supply appropriate errors for any unhandled cases.
	return "="
}
