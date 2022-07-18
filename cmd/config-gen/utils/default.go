package utils

import (
	"fmt"
	"time"

	pmodel "github.com/prometheus/common/model"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/config"
)

// DefaultConfig returns a configuration equivalent to the former
// pre-advanced-config settings.  This means that "normal" series labels
// will be of the form `<prefix><<.Resource>>`, cadvisor series will be
// of the form `container_`, and have the label `pod`.  Any series ending
// in total will be treated as a rate metric.
func DefaultConfig(rateInterval time.Duration, labelPrefix string) *config.MetricsDiscoveryConfig {
	return &config.MetricsDiscoveryConfig{
		Rules: []config.DiscoveryRule{
			// container seconds rate metrics
			{
				SeriesQuery: string(prom.MatchSeries("", prom.NameMatches("^container_.*"), prom.LabelNeq("container", "POD"), prom.LabelNeq("namespace", ""), prom.LabelNeq("pod", ""))),
				Resources: config.ResourceMapping{
					Overrides: map[string]config.GroupResource{
						"namespace": {Resource: "namespace"},
						"pod":       {Resource: "pod"},
					},
				},
				Name:         config.NameMapping{Matches: "^container_(.*)_seconds_total$"},
				MetricsQuery: fmt.Sprintf(`sum(rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[%s])) by (<<.GroupBy>>)`, pmodel.Duration(rateInterval).String()),
			},

			// container rate metrics
			{
				SeriesQuery:   string(prom.MatchSeries("", prom.NameMatches("^container_.*"), prom.LabelNeq("container", "POD"), prom.LabelNeq("namespace", ""), prom.LabelNeq("pod", ""))),
				SeriesFilters: []config.RegexFilter{{IsNot: "^container_.*_seconds_total$"}},
				Resources: config.ResourceMapping{
					Overrides: map[string]config.GroupResource{
						"namespace": {Resource: "namespace"},
						"pod":       {Resource: "pod"},
					},
				},
				Name:         config.NameMapping{Matches: "^container_(.*)_total$"},
				MetricsQuery: fmt.Sprintf(`sum(rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[%s])) by (<<.GroupBy>>)`, pmodel.Duration(rateInterval).String()),
			},

			// container non-cumulative metrics
			{
				SeriesQuery:   string(prom.MatchSeries("", prom.NameMatches("^container_.*"), prom.LabelNeq("container", "POD"), prom.LabelNeq("namespace", ""), prom.LabelNeq("pod", ""))),
				SeriesFilters: []config.RegexFilter{{IsNot: "^container_.*_total$"}},
				Resources: config.ResourceMapping{
					Overrides: map[string]config.GroupResource{
						"namespace": {Resource: "namespace"},
						"pod":       {Resource: "pod"},
					},
				},
				Name:         config.NameMapping{Matches: "^container_(.*)$"},
				MetricsQuery: `sum(<<.Series>>{<<.LabelMatchers>>,container!="POD"}) by (<<.GroupBy>>)`,
			},

			// normal non-cumulative metrics
			{
				SeriesQuery:   string(prom.MatchSeries("", prom.LabelNeq(fmt.Sprintf("%snamespace", labelPrefix), ""), prom.NameNotMatches("^container_.*"))),
				SeriesFilters: []config.RegexFilter{{IsNot: ".*_total$"}},
				Resources: config.ResourceMapping{
					Template: fmt.Sprintf("%s<<.Resource>>", labelPrefix),
				},
				MetricsQuery: "sum(<<.Series>>{<<.LabelMatchers>>}) by (<<.GroupBy>>)",
			},

			// normal rate metrics
			{
				SeriesQuery:   string(prom.MatchSeries("", prom.LabelNeq(fmt.Sprintf("%snamespace", labelPrefix), ""), prom.NameNotMatches("^container_.*"))),
				SeriesFilters: []config.RegexFilter{{IsNot: ".*_seconds_total"}},
				Name:          config.NameMapping{Matches: "^(.*)_total$"},
				Resources: config.ResourceMapping{
					Template: fmt.Sprintf("%s<<.Resource>>", labelPrefix),
				},
				MetricsQuery: fmt.Sprintf("sum(rate(<<.Series>>{<<.LabelMatchers>>}[%s])) by (<<.GroupBy>>)", pmodel.Duration(rateInterval).String()),
			},

			// seconds rate metrics
			{
				SeriesQuery: string(prom.MatchSeries("", prom.LabelNeq(fmt.Sprintf("%snamespace", labelPrefix), ""), prom.NameNotMatches("^container_.*"))),
				Name:        config.NameMapping{Matches: "^(.*)_seconds_total$"},
				Resources: config.ResourceMapping{
					Template: fmt.Sprintf("%s<<.Resource>>", labelPrefix),
				},
				MetricsQuery: fmt.Sprintf("sum(rate(<<.Series>>{<<.LabelMatchers>>}[%s])) by (<<.GroupBy>>)", pmodel.Duration(rateInterval).String()),
			},
		},

		ResourceRules: &config.ResourceRules{
			CPU: config.ResourceRule{
				ContainerQuery: fmt.Sprintf("sum(rate(container_cpu_usage_seconds_total{<<.LabelMatchers>>}[%s])) by (<<.GroupBy>>)", pmodel.Duration(rateInterval).String()),
				NodeQuery:      fmt.Sprintf("sum(1 - rate(node_cpu_seconds_total{<<.LabelMatchers>>, mode=\"idle\"}[%s])) by (<<.GroupBy>>)", pmodel.Duration(rateInterval).String()),
				Resources: config.ResourceMapping{
					Overrides: map[string]config.GroupResource{
						"namespace": {Resource: "namespace"},
						"pod":       {Resource: "pod"},
						"instance":  {Resource: "node"},
					},
				},
				ContainerLabel: fmt.Sprintf("%scontainer", labelPrefix),
			},
			Memory: config.ResourceRule{
				ContainerQuery: "sum(container_memory_working_set_bytes{<<.LabelMatchers>>}) by (<<.GroupBy>>)",
				NodeQuery:      "sum(node_memory_MemTotal_bytes{<<.LabelMatchers>>) - node_memory_MemAvailable_bytes{<<.LabelMatchers>>) by (<<.GroupBy>>)",
				Resources: config.ResourceMapping{
					Overrides: map[string]config.GroupResource{
						"namespace": {Resource: "namespace"},
						"pod":       {Resource: "pod"},
						"instance":  {Resource: "node"},
					},
				},
				ContainerLabel: fmt.Sprintf("%scontainer", labelPrefix),
			},
			Window: pmodel.Duration(rateInterval),
		},
	}
}
