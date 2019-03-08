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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/naming"
)

// SeriesConverter knows how to convert Prometheus series names and label names to
// metrics API resources, and vice-versa.  SeriesConverters should be safe to access
// concurrently.  Returned group-resources are "normalized" as per the
// MetricInfo#Normalized method.  Group-resources passed as arguments must
// themselves be normalized.
type SeriesConverter interface {
	// Selector produces the appropriate Prometheus series selector to match all
	// series handlable by this converter.
	Selector() prom.Selector
	// FilterSeries checks to see which of the given series match any additional
	// constrains beyond the series query.  It's assumed that the series given
	// already matche the series query.
	// FilterSeries(series []prom.Series) []prom.Series
	SeriesFilterer() SeriesFilterer
	ResourceConverter() naming.ResourceConverter

	QueryForExternalSeries(namespace string, series string, metricSelector labels.Selector) (prom.Selector, error)
	IdentifySeries(series prom.Series) (seriesIdentity, error)
}

type seriesIdentity struct {
	resources  []schema.GroupResource
	namespaced bool
	name       string
}

func (c *seriesConverter) Selector() prom.Selector {
	return c.seriesQuery
}

type seriesConverter struct {
	seriesQuery prom.Selector

	resourceConverter naming.ResourceConverter
	queryBuilder      QueryBuilder
	seriesFilterer    SeriesFilterer
	metricNamer       naming.MetricNamer
}

// queryTemplateArgs are the arguments for the metrics query template.
type queryTemplateArgs struct {
	Series            string
	LabelMatchers     string
	LabelValuesByName map[string][]string
	GroupBy           string
	GroupBySlice      []string
}

func (c *seriesConverter) IdentifySeries(series prom.Series) (seriesIdentity, error) {
	// TODO: warn if it doesn't match any resources

	resources, _ := c.resourceConverter.ResourcesForSeries(series)
	name, err := c.metricNamer.MetricNameForSeries(series)

	result := seriesIdentity{
		resources:  resources,
		namespaced: false,
		name:       name,
	}

	return result, err
}

func (c *seriesConverter) SeriesFilterer() SeriesFilterer {
	return c.seriesFilterer
}

func (c *seriesConverter) ResourceConverter() naming.ResourceConverter {
	return c.resourceConverter
}

func (c *seriesConverter) createQueryPartsFromSelector(metricSelector labels.Selector) []queryPart {
	requirements, _ := metricSelector.Requirements()

	selectors := []queryPart{}
	for i := 0; i < len(requirements); i++ {
		selector := c.convertRequirement(requirements[i])

		selectors = append(selectors, selector)
	}

	return selectors
}

func (c *seriesConverter) convertRequirement(requirement labels.Requirement) queryPart {
	labelName := requirement.Key()
	values := requirement.Values().List()

	return queryPart{
		labelName: labelName,
		values:    values,
		operator:  requirement.Operator(),
	}
}

type queryPart struct {
	labelName string
	values    []string
	operator  selection.Operator
}

func (c *seriesConverter) buildNamespaceQueryPartForSeries(namespace string) (queryPart, error) {
	result := queryPart{}

	// If we've been given a namespace, then we need to set up
	// the label requirements to target that namespace.
	if namespace != "default" {
		namespaceLbl, err := c.resourceConverter.LabelForResource(naming.NsGroupResource)
		if err != nil {
			return result, err
		}

		values := []string{namespace}

		result = queryPart{
			values:    values,
			labelName: string(namespaceLbl),
			operator:  selection.Equals,
		}
	}

	return result, nil
}

func (c *seriesConverter) buildResourceQueryPartForSeries(resource schema.GroupResource, names ...string) (queryPart, error) {
	result := queryPart{}

	// If we've been given a resource, then we need to set up
	// the label requirements to target that resource.
	resourceLbl, err := c.resourceConverter.LabelForResource(resource)
	if err != nil {
		return result, err
	}

	result = queryPart{
		labelName: string(resourceLbl),
		values:    names,
		operator:  selection.Equals,
	}

	return result, nil
}

func (c *seriesConverter) QueryForSeries(series string, resource schema.GroupResource, namespace string, names ...string) (prom.Selector, error) {
	queryParts := []queryPart{}

	// Build up the namespace part of the query.
	namespaceQueryPart, err := c.buildNamespaceQueryPartForSeries(namespace)
	if err != nil {
		return "", err
	}

	if namespaceQueryPart.labelName != "" {
		queryParts = append(queryParts, namespaceQueryPart)
	}

	// Build up the resource part of the query.
	resourceQueryPart, err := c.buildResourceQueryPartForSeries(resource, names...)
	if err != nil {
		return "", err
	}

	if resourceQueryPart.labelName != "" {
		queryParts = append(queryParts, resourceQueryPart)
	}

	return c.queryBuilder.BuildSelector(series, resourceQueryPart.labelName, []string{resourceQueryPart.labelName}, queryParts)
}

// ConvertersFromConfig produces a MetricNamer for each rule in the given config.
func ConvertersFromConfig(cfg *config.MetricsDiscoveryConfig, mapper apimeta.RESTMapper) ([]SeriesConverter, []error) {
	errs := []error{}
	converters := []SeriesConverter{}

	for _, rule := range cfg.ExternalRules {
		if externalConverter, err := converterFromRule(rule, mapper); err == nil {
			converters = append(converters, externalConverter)
		} else {
			errs = append(errs, err)
		}
	}
	return converters, errs
}

func converterFromRule(rule config.DiscoveryRule, mapper apimeta.RESTMapper) (SeriesConverter, error) {
	var (
		err error
	)

	resourceConverter, err := naming.NewResourceConverter(rule.Resources.Template, rule.Resources.Overrides, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to create ResourceConverter associated with series query %q: %v", rule.SeriesQuery, err)
	}

	queryBuilder, err := NewQueryBuilder(rule.MetricsQuery)
	if err != nil {
		return nil, fmt.Errorf("unable to create a QueryBuilder associated with series query %q: %v", rule.SeriesQuery, err)
	}

	seriesFilterer, err := NewSeriesFilterer(rule.SeriesFilters)
	if err != nil {
		return nil, fmt.Errorf("unable to create a SeriesFilter associated with series query %q: %v", rule.SeriesQuery, err)
	}

	if rule.Name.Matches != "" {
		err := seriesFilterer.AddRequirement(config.RegexFilter{Is: rule.Name.Matches})
		if err != nil {
			return nil, fmt.Errorf("unable to apply the series name filter from name rules associated with series query %q: %v", rule.SeriesQuery, err)
		}
	}

	metricNamer, err := naming.NewMetricNamer(rule.Name)
	if err != nil {
		return nil, fmt.Errorf("unable to create a MetricNamer associated with series query %q: %v", rule.SeriesQuery, err)
	}

	return &seriesConverter{
		seriesQuery:       prom.Selector(rule.SeriesQuery),
		resourceConverter: resourceConverter,
		queryBuilder:      queryBuilder,
		seriesFilterer:    seriesFilterer,
		metricNamer:       metricNamer,
	}, nil
}

func (c *seriesConverter) buildNamespaceQueryPartForExternalSeries(namespace string) (queryPart, error) {
	namespaceLbl, err := c.resourceConverter.LabelForResource(naming.NsGroupResource)

	return queryPart{
		labelName: string(namespaceLbl),
		values:    []string{namespace},
		operator:  selection.Equals,
	}, err
}

func (c *seriesConverter) QueryForExternalSeries(namespace string, series string, metricSelector labels.Selector) (prom.Selector, error) {
	queryParts := []queryPart{}

	if namespace != "default" {
		// Build up the namespace part of the query.
		namespaceQueryPart, err := c.buildNamespaceQueryPartForExternalSeries(namespace)
		if err != nil {
			return "", err
		}

		queryParts = append(queryParts, namespaceQueryPart)
	}

	// Build up the query parts from the selector.
	queryParts = append(queryParts, c.createQueryPartsFromSelector(metricSelector)...)

	selector, err := c.queryBuilder.BuildSelector(series, "", []string{}, queryParts)
	if err != nil {
		return "", err
	}

	return selector, nil
}
