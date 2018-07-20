package provider

import (
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

var nsGroupResource = schema.GroupResource{Resource: "namespaces"}

// MetricNamer knows how to convert Prometheus series names and label names to
// metrics API resources, and vice-versa.  MetricNamers should be safe to access
// concurrently.  Returned group-resources are "normalized" as per the
// MetricInfo#Normalized method.  Group-resources passed as arguments must
// themselves be normalized.
type MetricNamer interface {
	// Selector produces the appropriate Prometheus series selector to match all
	// series handlable by this namer.
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

func (n *metricNamer) Selector() prom.Selector {
	return n.seriesQuery
}

type metricNamer struct {
	seriesQuery prom.Selector

	resourceConverter   ResourceConverter
	queryBuilder        QueryBuilder
	seriesFilterer      SeriesFilterer
	metricNameConverter MetricNameConverter
	mapper              apimeta.RESTMapper

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

func (n *metricNamer) MetricType() config.MetricType {
	return n.metricType
}

func (n *metricNamer) ExternalMetricNamespaceLabelName() string {
	return n.externalMetricNamespaceLabel
}

func (n *metricNamer) IdentifySeries(series prom.Series) (seriesIdentity, error) {
	// TODO: warn if it doesn't match any resources
	resources, namespaced := n.resourceConverter.ResourcesForSeries(series)
	name, err := n.metricNameConverter.GetMetricNameForSeries(series)

	result := seriesIdentity{
		resources:  resources,
		namespaced: namespaced,
		name:       name,
	}

	return result, err
}

func (n *metricNamer) SeriesFilterer() SeriesFilterer {
	return n.seriesFilterer
}

func (n *metricNamer) ResourceConverter() ResourceConverter {
	return n.resourceConverter
}

func (n *metricNamer) createQueryPartsFromSelector(metricSelector labels.Selector) []queryPart {
	requirements, _ := metricSelector.Requirements()

	selectors := []queryPart{}
	for i := 0; i < len(requirements); i++ {
		selector := n.convertRequirement(requirements[i])

		selectors = append(selectors, selector)
	}

	return selectors
}

func (n *metricNamer) convertRequirement(requirement labels.Requirement) queryPart {
	labelName := requirement.Key()
	values := requirement.Values().List()

	return queryPart{
		labelName: labelName,
		values:    values,
	}
}

type queryPart struct {
	labelName string
	values    []string
}

func (n *metricNamer) buildNamespaceQueryPartForSeries(namespace string) (queryPart, error) {
	result := queryPart{}

	//If we've been given a namespace, then we need to set up
	//the label requirements to target that namespace.
	if namespace != "" {
		namespaceLbl, err := n.ResourceConverter().LabelForResource(nsGroupResource)
		if err != nil {
			return result, err
		}

		values := []string{namespace}

		result = queryPart{
			values:    values,
			labelName: string(namespaceLbl),
		}
	}

	return result, nil
}

func (n *metricNamer) buildResourceQueryPartForSeries(resource schema.GroupResource, names ...string) (queryPart, error) {
	result := queryPart{}

	//If we've been given a resource, then we need to set up
	//the label requirements to target that resource.
	resourceLbl, err := n.ResourceConverter().LabelForResource(resource)
	if err != nil {
		return result, err
	}

	result = queryPart{
		labelName: string(resourceLbl),
		values:    names,
	}

	return result, nil
}

func (n *metricNamer) QueryForSeries(series string, resource schema.GroupResource, namespace string, names ...string) (prom.Selector, error) {
	queryParts := []queryPart{}

	//Build up the namespace part of the query.
	namespaceQueryPart, err := n.buildNamespaceQueryPartForSeries(namespace)
	if err != nil {
		return "", err
	}

	queryParts = append(queryParts, namespaceQueryPart)

	//Build up the resource part of the query.
	resourceQueryPart, err := n.buildResourceQueryPartForSeries(resource, names...)
	if err != nil {
		return "", err
	}

	queryParts = append(queryParts, resourceQueryPart)

	return n.queryBuilder.BuildSelector(series, resourceQueryPart.labelName, []string{resourceQueryPart.labelName}, queryParts)
}

// NamersFromConfig produces a MetricNamer for each rule in the given config.
func NamersFromConfig(cfg *config.MetricsDiscoveryConfig, mapper apimeta.RESTMapper) ([]MetricNamer, error) {
	namers := make([]MetricNamer, len(cfg.Rules))
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

		metricNameConverter, err := NewMetricNameConverter(rule.Name)
		if err != nil {
			return nil, fmt.Errorf("unable to create a MetricNameConverter associated with series query %q: %v", rule.SeriesQuery, err)
		}

		namespaceLabel := ""
		if rule.MetricType == config.External {
			namespaceLabel = rule.ExternalMetricNamespaceLabelName
		}

		metricType := rule.MetricType
		if metricType == config.MetricType("") {
			metricType = config.Custom
		}

		namer := &metricNamer{
			seriesQuery: prom.Selector(rule.SeriesQuery),
			mapper:      mapper,

			resourceConverter:            resourceConverter,
			queryBuilder:                 queryBuilder,
			seriesFilterer:               seriesFilterer,
			metricNameConverter:          metricNameConverter,
			metricType:                   metricType,
			externalMetricNamespaceLabel: namespaceLabel,
		}

		namers[i] = namer
	}

	return namers, nil
}

func (n *metricNamer) buildNamespaceQueryPartForExternalSeries(namespace string) (queryPart, error) {
	return queryPart{
		labelName: n.externalMetricNamespaceLabel,
		values:    []string{namespace},
	}, nil
}

func (n *metricNamer) QueryForExternalSeries(namespace string, series string, metricSelector labels.Selector) (prom.Selector, error) {
	queryParts := []queryPart{}

	if namespace != "" {
		//Build up the namespace part of the query.
		namespaceQueryPart, err := n.buildNamespaceQueryPartForExternalSeries(namespace)
		if err != nil {
			return "", err
		}

		queryParts = append(queryParts, namespaceQueryPart)
	}

	//Build up the query parts from the selector.
	queryParts = append(queryParts, n.createQueryPartsFromSelector(metricSelector)...)

	selector, err := n.queryBuilder.BuildSelector(series, "", []string{}, queryParts)
	if err != nil {
		return "", err
	}

	return selector, nil
}
