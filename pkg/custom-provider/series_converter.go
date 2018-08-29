package provider

import (
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

var nsGroupResource = schema.GroupResource{Resource: "namespaces"}

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
	ResourceConverter() ResourceConverter

	// MetricNameForSeries returns the name (as presented in the API) for a given series.
	// MetricNameForSeries(series prom.Series) (string, error)
	// QueryForSeries returns the query for a given series (not API metric name), with
	// the given namespace name (if relevant), resource, and resource names.
	QueryForSeries(series string, resource schema.GroupResource, namespace string, names ...string) (prom.Selector, error)
	QueryForExternalSeries(namespace string, series string, metricSelector labels.Selector) (prom.Selector, error)
	IdentifySeries(series prom.Series) (seriesIdentity, error)
	MetricType() config.MetricType
	ExternalMetricNamespaceLabelName() string
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

	resourceConverter ResourceConverter
	queryBuilder      QueryBuilder
	seriesFilterer    SeriesFilterer
	metricNamer       MetricNamer
	mapper            apimeta.RESTMapper

	metricType                   config.MetricType
	externalMetricNamespaceLabel string
}

// queryTemplateArgs are the arguments for the metrics query template.
type queryTemplateArgs struct {
	Series            string
	LabelMatchers     string
	LabelValuesByName map[string][]string
	GroupBy           string
	GroupBySlice      []string
}

func (c *seriesConverter) MetricType() config.MetricType {
	return c.metricType
}

func (c *seriesConverter) ExternalMetricNamespaceLabelName() string {
	return c.externalMetricNamespaceLabel
}

func (c *seriesConverter) IdentifySeries(series prom.Series) (seriesIdentity, error) {
	// TODO: warn if it doesn't match any resources
	resources, namespaced := c.resourceConverter.ResourcesForSeries(series)
	name, err := c.metricNamer.GetMetricNameForSeries(series)

	result := seriesIdentity{
		resources:  resources,
		namespaced: namespaced,
		name:       name,
	}

	return result, err
}

func (c *seriesConverter) SeriesFilterer() SeriesFilterer {
	return c.seriesFilterer
}

func (c *seriesConverter) ResourceConverter() ResourceConverter {
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
	if namespace != "" {
		namespaceLbl, err := c.resourceConverter.LabelForResource(nsGroupResource)
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
func ConvertersFromConfig(cfg *config.MetricsDiscoveryConfig, mapper apimeta.RESTMapper) ([]SeriesConverter, error) {
	converters := make([]SeriesConverter, len(cfg.Rules))
	for i, rule := range cfg.Rules {
		var err error

		resourceConverter, err := NewResourceConverter(rule.Resources.Template, rule.Resources.Overrides, mapper)
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

		metricNamer, err := NewMetricNamer(rule.Name)
		if err != nil {
			return nil, fmt.Errorf("unable to create a MetricNamer associated with series query %q: %v", rule.SeriesQuery, err)
		}

		namespaceLabel := ""
		if rule.MetricType == config.External {
			namespaceLabel = rule.ExternalMetricNamespaceLabelName
		}

		metricType := rule.MetricType
		if metricType == config.MetricType("") {
			metricType = config.Custom
		}

		converter := &seriesConverter{
			seriesQuery: prom.Selector(rule.SeriesQuery),
			mapper:      mapper,

			resourceConverter:            resourceConverter,
			queryBuilder:                 queryBuilder,
			seriesFilterer:               seriesFilterer,
			metricNamer:                  metricNamer,
			metricType:                   metricType,
			externalMetricNamespaceLabel: namespaceLabel,
		}

		converters[i] = converter
	}

	return converters, nil
}

func (c *seriesConverter) buildNamespaceQueryPartForExternalSeries(namespace string) (queryPart, error) {
	return queryPart{
		labelName: c.externalMetricNamespaceLabel,
		values:    []string{namespace},
		operator:  selection.Equals,
	}, nil
}

func (c *seriesConverter) QueryForExternalSeries(namespace string, series string, metricSelector labels.Selector) (prom.Selector, error) {
	queryParts := []queryPart{}

	if namespace != "" {
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
