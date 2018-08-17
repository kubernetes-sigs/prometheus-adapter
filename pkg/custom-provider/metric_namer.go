package provider

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
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

func (r *metricNamer) Selector() prom.Selector {
	return r.seriesQuery
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
	seriesQuery          prom.Selector
	metricsQueryTemplate *template.Template
	nameMatches          *regexp.Regexp
	nameAs               string
	seriesMatchers       []*reMatcher

	naming.ResourceConverter
}

// queryTemplateArgs are the arguments for the metrics query template.
type queryTemplateArgs struct {
	Series            string
	LabelMatchers     string
	LabelValuesByName map[string][]string
	GroupBy           string
	GroupBySlice      []string
}

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
	var exprs []string
	valuesByName := map[string][]string{}

	if namespace != "" {
		namespaceLbl, err := n.LabelForResource(nsGroupResource)
		if err != nil {
			return "", err
		}
		exprs = append(exprs, prom.LabelEq(string(namespaceLbl), namespace))
		valuesByName[string(namespaceLbl)] = []string{namespace}
	}

	resourceLbl, err := n.LabelForResource(resource)
	if err != nil {
		return "", err
	}
	matcher := prom.LabelEq
	targetValue := names[0]
	if len(names) > 1 {
		matcher = prom.LabelMatches
		targetValue = strings.Join(names, "|")
	}
	exprs = append(exprs, matcher(string(resourceLbl), targetValue))
	valuesByName[string(resourceLbl)] = names

	args := queryTemplateArgs{
		Series:            series,
		LabelMatchers:     strings.Join(exprs, ","),
		LabelValuesByName: valuesByName,
		GroupBy:           string(resourceLbl),
		GroupBySlice:      []string{string(resourceLbl)},
	}
	queryBuff := new(bytes.Buffer)
	if err := n.metricsQueryTemplate.Execute(queryBuff, args); err != nil {
		return "", err
	}

	if queryBuff.Len() == 0 {
		return "", fmt.Errorf("empty query produced by metrics query template")
	}

	return prom.Selector(queryBuff.String()), nil
}

func (n *metricNamer) MetricNameForSeries(series prom.Series) (string, error) {
	matches := n.nameMatches.FindStringSubmatchIndex(series.Name)
	if matches == nil {
		return "", fmt.Errorf("series name %q did not match expected pattern %q", series.Name, n.nameMatches.String())
	}
	outNameBytes := n.nameMatches.ExpandString(nil, n.nameAs, series.Name, matches)
	return string(outNameBytes), nil
}

// NamersFromConfig produces a MetricNamer for each rule in the given config.
func NamersFromConfig(cfg *config.MetricsDiscoveryConfig, mapper apimeta.RESTMapper) ([]MetricNamer, error) {
	namers := make([]MetricNamer, len(cfg.Rules))

	for i, rule := range cfg.Rules {
		metricsQueryTemplate, err := template.New("metrics-query").Delims("<<", ">>").Parse(rule.MetricsQuery)
		if err != nil {
			return nil, fmt.Errorf("unable to parse metrics query template %q associated with series query %q: %v", rule.MetricsQuery, rule.SeriesQuery, err)
		}

		seriesMatchers := make([]*reMatcher, len(rule.SeriesFilters))
		for i, filterRaw := range rule.SeriesFilters {
			matcher, err := newReMatcher(filterRaw)
			if err != nil {
				return nil, fmt.Errorf("unable to generate series name filter associated with series query %q: %v", rule.SeriesQuery, err)
			}
			seriesMatchers[i] = matcher
		}
		if rule.Name.Matches != "" {
			matcher, err := newReMatcher(config.RegexFilter{Is: rule.Name.Matches})
			if err != nil {
				return nil, fmt.Errorf("unable to generate series name filter from name rules associated with series query %q: %v", rule.SeriesQuery, err)
			}
			seriesMatchers = append(seriesMatchers, matcher)
		}

		var nameMatches *regexp.Regexp
		if rule.Name.Matches != "" {
			nameMatches, err = regexp.Compile(rule.Name.Matches)
			if err != nil {
				return nil, fmt.Errorf("unable to compile series name match expression %q associated with series query %q: %v", rule.Name.Matches, rule.SeriesQuery, err)
			}
		} else {
			// this will always succeed
			nameMatches = regexp.MustCompile(".*")
		}
		nameAs := rule.Name.As
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
				return nil, fmt.Errorf("must specify an 'as' value for name matcher %q associated with series query %q", rule.Name.Matches, rule.SeriesQuery)
			}
		}

		resConv, err := naming.NewResourceConverter(rule.Resources.Template, rule.Resources.Overrides, mapper)
		if err != nil {
			return nil, err
		}

		namer := &metricNamer{
			seriesQuery:          prom.Selector(rule.SeriesQuery),
			metricsQueryTemplate: metricsQueryTemplate,
			nameMatches:          nameMatches,
			nameAs:               nameAs,
			seriesMatchers:       seriesMatchers,
			ResourceConverter:    resConv,
		}

		namers[i] = namer
	}

	return namers, nil
}
