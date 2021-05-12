package config

import (
	pmodel "github.com/prometheus/common/model"
)

type MetricsDiscoveryConfig struct {
	// Rules specifies how to discover and map Prometheus metrics to
	// custom metrics API resources.  The rules are applied independently,
	// and thus must be mutually exclusive.  Rules with the same SeriesQuery
	// will make only a single API call.
	Rules         []DiscoveryRule `json:"rules" yaml:"rules"`
	ResourceRules *ResourceRules  `json:"resourceRules,omitempty" yaml:"resourceRules,omitempty"`
	ExternalRules []DiscoveryRule `json:"externalRules,omitempty" yaml:"externalRules,omitempty"`
}

// DiscoveryRule describes a set of rules for transforming Prometheus metrics to/from
// custom metrics API resources.
type DiscoveryRule struct {
	// SeriesQuery specifies which metrics this rule should consider via a Prometheus query
	// series selector query.
	SeriesQuery string `json:"seriesQuery" yaml:"seriesQuery"`
	// SeriesFilters specifies additional regular expressions to be applied on
	// the series names returned from the query.  This is useful for constraints
	// that can't be represented in the SeriesQuery (e.g. series matching `container_.+`
	// not matching `container_.+_total`.  A filter will be automatically appended to
	// match the form specified in Name.
	SeriesFilters []RegexFilter `json:"seriesFilters" yaml:"seriesFilters"`
	// Resources specifies how associated Kubernetes resources should be discovered for
	// the given metrics.
	Resources ResourceMapping `json:"resources" yaml:"resources"`
	// Name specifies how the metric name should be transformed between custom metric
	// API resources, and Prometheus metric names.
	Name NameMapping `json:"name" yaml:"name"`
	// MetricsQuery specifies modifications to the metrics query, such as converting
	// cumulative metrics to rate metrics.  It is a template where `.LabelMatchers` is
	// a the comma-separated base label matchers and `.Series` is the series name, and
	// `.GroupBy` is the comma-separated expected group-by label names. The delimeters
	// are `<<` and `>>`.
	MetricsQuery string `json:"metricsQuery,omitempty" yaml:"metricsQuery,omitempty"`
}

// RegexFilter is a filter that matches positively or negatively against a regex.
// Only one field may be set at a time.
type RegexFilter struct {
	Is    string `json:"is,omitempty" yaml:"is,omitempty"`
	IsNot string `json:"isNot,omitempty" yaml:"isNot,omitempty"`
}

// ResourceMapping specifies how to map Kubernetes resources to Prometheus labels
type ResourceMapping struct {
	// Template specifies a golang string template for converting a Kubernetes
	// group-resource to a Prometheus label.  The template object contains
	// the `.Group` and `.Resource` fields.  The `.Group` field will have
	// dots replaced with underscores, and the `.Resource` field will be
	// singularized.  The delimiters are `<<` and `>>`.
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
	// Overrides specifies exceptions to the above template, mapping label names
	// to group-resources
	Overrides map[string]GroupResource `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	// Namespaced ignores the source namespace of the requester and requires one in the query
	Namespaced *bool `json:"namespaced,omitempty" yaml:"namespaced,omitempty"`
}

// GroupResource represents a Kubernetes group-resource.
type GroupResource struct {
	Group    string `json:"group,omitempty" yaml:"group,omitempty"`
	Resource string `json:"resource" yaml:"resource"`
}

// NameMapping specifies how to convert Prometheus metrics
// to/from custom metrics API resources.
type NameMapping struct {
	// Matches is a regular expression that is used to match
	// Prometheus series names.  It may be left blank, in which
	// case it is equivalent to `.*`.
	Matches string `json:"matches" yaml:"matches"`
	// As is the name used in the API.  Captures from Matches
	// are available for use here.  If not specified, it defaults
	// to $0 if no capture groups are present in Matches, or $1
	// if only one is present, and will error if multiple are.
	As string `json:"as" yaml:"as"`
}

// ResourceRules describe the rules for querying resource metrics
// API results.  It's assumed that the same metrics can be used
// to aggregate across different resources.
type ResourceRules struct {
	CPU    ResourceRule `json:"cpu" yaml:"cpu"`
	Memory ResourceRule `json:"memory" yaml:"memory"`
	// Window is the window size reported by the resource metrics API.  It should match the value used
	// in your containerQuery and nodeQuery if you use a `rate` function.
	Window pmodel.Duration `json:"window" yaml:"window"`
}

// ResourceRule describes how to query metrics for some particular
// system resource metric.
type ResourceRule struct {
	// Container is the query used to fetch the metrics for containers.
	ContainerQuery string `json:"containerQuery" yaml:"containerQuery"`
	// NodeQuery is the query used to fetch the metrics for nodes
	// (for instance, simply aggregating by node label is insufficient for
	// cadvisor metrics -- you need to select the `/` container).
	NodeQuery string `json:"nodeQuery" yaml:"nodeQuery"`
	// Resources specifies how associated Kubernetes resources should be discovered for
	// the given metrics.
	Resources ResourceMapping `json:"resources" yaml:"resources"`
	// ContainerLabel indicates the name of the Prometheus label containing the container name
	// (since "container" is not a resource, this can't go in the `resources` block, but is similar).
	ContainerLabel string `json:"containerLabel" yaml:"containerLabel"`
}
