package provider

import (
	"fmt"
	s "strings"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type ExternalMetricQueryBuilder interface {
	BuildPrometheusQuery(namespace string, metricName string, metricSelector labels.Selector) string
}

type externalMetricQueryBuilder struct {
}

func NewExternalMetricQueryBuilder() ExternalMetricQueryBuilder {
	return &externalMetricQueryBuilder{}
}

func (p *externalMetricQueryBuilder) BuildPrometheusQuery(namespace string, metricName string, metricSelector labels.Selector) string {
	namespaceSelector := p.makeLabelFilter("namespace", "=", namespace)
	otherSelectors := p.convertSelectors(metricSelector)

	finalTargets := append([]string{namespaceSelector}, otherSelectors...)
	joinedLabels := s.Join(finalTargets, ", ")
	return fmt.Sprintf("%s{%s}", metricName, joinedLabels)
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
		return "=~"
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
