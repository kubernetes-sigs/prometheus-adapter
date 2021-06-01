/*
Copyright 2019 The Kubernetes Authors.

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

package naming

import (
	"fmt"
	"regexp"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/config"
)

// MetricNamer knows how to convert Prometheus series names and label names to
// metrics API resources, and vice-versa.  MetricNamers should be safe to access
// concurrently.  Returned group-resources are "normalized" as per the
// MetricInfo#Normalized method.  Group-resources passed as arguments must
// themselves be normalized.
type MetricNamer interface {
	// Selector produces the appropriate Prometheus series selector to match all
	// series handable by this namer.
	Selector() prom.Selector
	// FilterSeries checks to see which of the given series match any additional
	// constraints beyond the series query.  It's assumed that the series given
	// already match the series query.
	FilterSeries(series []prom.Series) []prom.Series
	// MetricNameForSeries returns the name (as presented in the API) for a given series.
	MetricNameForSeries(series prom.Series) (string, error)
	// QueryForSeries returns the query for a given series (not API metric name), with
	// the given namespace name (if relevant), resource, and resource names.
	QueryForSeries(series string, resource schema.GroupResource, namespace string, metricSelector labels.Selector, names ...string) (prom.Selector, error)
	// QueryForExternalSeries returns the query for a given series (not API metric name), with
	// the given namespace name (if relevant), resource, and resource names.
	QueryForExternalSeries(series string, namespace string, targetLabels labels.Selector) (prom.Selector, error)

	ResourceConverter
}

func (n *metricNamer) Selector() prom.Selector {
	return n.seriesQuery
}

// ReMatcher either positively or negatively matches a regex
type ReMatcher struct {
	regex    *regexp.Regexp
	positive bool
}

func NewReMatcher(cfg config.RegexFilter) (*ReMatcher, error) {
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

	return &ReMatcher{
		regex:    regex,
		positive: positive,
	}, nil
}

func (m *ReMatcher) Matches(val string) bool {
	return m.regex.MatchString(val) == m.positive
}

type metricNamer struct {
	seriesQuery    prom.Selector
	metricsQuery   MetricsQuery
	nameMatches    *regexp.Regexp
	nameAs         string
	seriesMatchers []*ReMatcher

	ResourceConverter
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

func (n *metricNamer) QueryForSeries(series string, resource schema.GroupResource, namespace string, metricSelector labels.Selector, names ...string) (prom.Selector, error) {
	return n.metricsQuery.Build(series, resource, namespace, nil, metricSelector, names...)
}

func (n *metricNamer) QueryForExternalSeries(series string, namespace string, metricSelector labels.Selector) (prom.Selector, error) {
	return n.metricsQuery.BuildExternal(series, namespace, "", []string{}, metricSelector)
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
func NamersFromConfig(cfg []config.DiscoveryRule, mapper apimeta.RESTMapper) ([]MetricNamer, error) {
	namers := make([]MetricNamer, len(cfg))

	for i, rule := range cfg {
		resConv, err := NewResourceConverter(rule.Resources.Template, rule.Resources.Overrides, mapper)
		if err != nil {
			return nil, err
		}

		// queries are namespaced by default unless the rule specifically disables it
		namespaced := true
		if rule.Resources.Namespaced != nil {
			namespaced = *rule.Resources.Namespaced
		}

		metricsQuery, err := NewExternalMetricsQuery(rule.MetricsQuery, resConv, namespaced)
		if err != nil {
			return nil, fmt.Errorf("unable to construct metrics query associated with series query %q: %v", rule.SeriesQuery, err)
		}

		seriesMatchers := make([]*ReMatcher, len(rule.SeriesFilters))
		for i, filterRaw := range rule.SeriesFilters {
			matcher, err := NewReMatcher(filterRaw)
			if err != nil {
				return nil, fmt.Errorf("unable to generate series name filter associated with series query %q: %v", rule.SeriesQuery, err)
			}
			seriesMatchers[i] = matcher
		}
		if rule.Name.Matches != "" {
			matcher, err := NewReMatcher(config.RegexFilter{Is: rule.Name.Matches})
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

		namer := &metricNamer{
			seriesQuery:       prom.Selector(rule.SeriesQuery),
			metricsQuery:      metricsQuery,
			nameMatches:       nameMatches,
			nameAs:            nameAs,
			seriesMatchers:    seriesMatchers,
			ResourceConverter: resConv,
		}

		namers[i] = namer
	}

	return namers, nil
}
