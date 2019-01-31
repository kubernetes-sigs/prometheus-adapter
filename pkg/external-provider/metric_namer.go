package provider

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/naming"
)

var nsGroupResource = schema.GroupResource{Resource: "namespaces"}
var groupNameSanitizer = strings.NewReplacer(".", "_", "-", "_")

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
	FilterSeries(series []prom.Series) []prom.Series
	// MetricNameForSeries returns the name (as presented in the API) for a given series.
	MetricNameForSeries(series prom.Series) (string, error)
	// QueryForSeries returns the query for a given series (not API metric name), with
	// the given namespace name (if relevant), resource, and resource names.
	QueryForSeries(series string, resource schema.GroupResource, namespace string, names ...string) (prom.Selector, error)

	naming.ResourceConverter
}

func (n *metricNamer) Selector() prom.Selector {
	return n.seriesQuery
}

// reMatcher either positively or negatively matches a regex
type reMatcher struct {
	regex    *regexp.Regexp
	positive bool
}

func newReMatcher(cfg config.RegexFilter) (*reMatcher, error) {
	if cfg.Is != "" && cfg.IsNot != "" {
		return nil, fmt.Errorf("cannot have both an `is` (%q) and `isNot` (%q) expression in a single filter", cfg.Is, cfg.IsNot)
	}
	if cfg.Is == "" && cfg.IsNot == "" {
		return nil, fmt.Errorf("must have either an `is` or `isNot` expression in a filter")
	}

	var positive bool
	var regexRaw string
	if cfg.Is != "" {
		positive = true
		regexRaw = cfg.Is
	} else {
		positive = false
		regexRaw = cfg.IsNot
	}

	regex, err := regexp.Compile(regexRaw)
	if err != nil {
		return nil, fmt.Errorf("unable to compile series filter %q: %v", regexRaw, err)
	}

	return &reMatcher{
		regex:    regex,
		positive: positive,
	}, nil
}

func (m *reMatcher) Matches(val string) bool {
	return m.regex.MatchString(val) == m.positive
}

type metricNamer struct {
	seriesQuery    prom.Selector
	metricsQuery   naming.MetricsQuery
	nameMatches    *regexp.Regexp
	nameAs         string
	seriesMatchers []*reMatcher

	naming.ResourceConverter
}

// queryTemplateArgs are the arguments for the metrics query template.
func (n *metricNamer) FilterSeries(initialSeries []prom.Series) []prom.Series {
	if len(n.seriesMatchers) == 0 {
		return initialSeries
	}

	finalSeries := make([]prom.Series, 0, len(initialSeries))
SeriesLoop:
	for _, series := range initialSeries {
		for _, matcher := range n.seriesMatchers {
			if !matcher.Matches(series.Name) {
				continue SeriesLoop
			}
		}
		finalSeries = append(finalSeries, series)
	}

	return finalSeries
}

func (n *metricNamer) QueryForSeries(series string, resource schema.GroupResource, namespace string, names ...string) (prom.Selector, error) {
	return n.metricsQuery.Build(series, resource, namespace, nil, names...)
}

func (n *metricNamer) MetricNameForSeries(series prom.Series) (string, error) {
	matches := n.nameMatches.FindStringSubmatchIndex(series.Name)
	if matches == nil {
		return "", fmt.Errorf("series name %q did not match expected pattern %q", series.Name, n.nameMatches.String())
	}
	outNameBytes := n.nameMatches.ExpandString(nil, n.nameAs, series.Name, matches)
	return string(outNameBytes), nil
}

// NewMetricNamer creates a MetricNamer capable of translating Prometheus series names
// into custom metric names.
func NewMetricNamer(mapping config.NameMapping) (MetricNamer, error) {
	var nameMatches *regexp.Regexp
	var err error
	if mapping.Matches != "" {
		nameMatches, err = regexp.Compile(mapping.Matches)
		if err != nil {
			return nil, fmt.Errorf("unable to compile series name match expression %q: %v", mapping.Matches, err)
		}
	} else {
		// this will always succeed
		nameMatches = regexp.MustCompile(".*")
	}
	nameAs := mapping.As
	if nameAs == "" {
		// check if we have an obvious default
		subexpNames := nameMatches.SubexpNames()
		if len(subexpNames) == 1 {
			// no capture groups, use the whole thing
			nameAs = "$0"
		} else if len(subexpNames) == 2 {
			// one capture group, use that
			nameAs = "$1"
		} else {
			return nil, fmt.Errorf("must specify an 'as' value for name matcher %q", mapping.Matches)
		}
	}

	return &metricNamer{
		nameMatches: nameMatches,
		nameAs:      nameAs,
	}, nil
}
