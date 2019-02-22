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

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/naming"
)

// SeriesFilterer provides functions for filtering collections of Prometheus series
// to only those that meet certain requirements.
type SeriesFilterer interface {
	FilterSeries(series []prom.Series) []prom.Series
	AddRequirement(filter config.RegexFilter) error
}

type seriesFilterer struct {
	seriesMatchers []*naming.ReMatcher
}

// NewSeriesFilterer creates a SeriesFilterer that will remove any series that do not
// meet the requirements of the provided RegexFilter(s).
func NewSeriesFilterer(filters []config.RegexFilter) (SeriesFilterer, error) {
	seriesMatchers := make([]*naming.ReMatcher, len(filters))
	for i, filterRaw := range filters {
		matcher, err := naming.NewReMatcher(filterRaw)
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
	matcher, err := naming.NewReMatcher(filterRaw)
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
