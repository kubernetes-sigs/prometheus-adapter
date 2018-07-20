package provider

import (
	"fmt"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

//SeriesFilterer provides functions for filtering collections of Prometheus series
//to only those that meet certain requirements.
type SeriesFilterer interface {
	FilterSeries(series []prom.Series) []prom.Series
	AddRequirement(filter config.RegexFilter) error
}

type seriesFilterer struct {
	seriesMatchers []*reMatcher
}

//NewSeriesFilterer creates a SeriesFilterer that will remove any series that do not
//meet the requirements of the provided RegexFilter(s).
func NewSeriesFilterer(filters []config.RegexFilter) (SeriesFilterer, error) {
	seriesMatchers := make([]*reMatcher, len(filters))
	for i, filterRaw := range filters {
		matcher, err := newReMatcher(filterRaw)
		if err != nil {
			return nil, fmt.Errorf("unable to generate series name filter: %v", err)
		}
		seriesMatchers[i] = matcher
	}

	return &seriesFilterer{
		seriesMatchers: seriesMatchers,
	}, nil
}

func (n *seriesFilterer) AddRequirement(filterRaw config.RegexFilter) error {
	matcher, err := newReMatcher(filterRaw)
	if err != nil {
		return fmt.Errorf("unable to generate series name filter: %v", err)
	}

	n.seriesMatchers = append(n.seriesMatchers, matcher)
	return nil
}

func (n *seriesFilterer) FilterSeries(initialSeries []prom.Series) []prom.Series {
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
