/*
Copyright 2018 The Kubernetes Authors.

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

package fake

import (
	"context"
	"fmt"

	pmodel "github.com/prometheus/common/model"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
)

// FakePrometheusClient is a fake instance of prom.Client
type FakePrometheusClient struct {
	// AcceptableInterval is the interval in which to return queries
	AcceptableInterval pmodel.Interval
	// ErrQueries are queries that result in an error (whether from Query or Series)
	ErrQueries map[prom.Selector]error
	// Series are non-error responses to partial Series calls
	SeriesResults map[prom.Selector][]prom.Series
	// QueryResults are non-error responses to Query
	QueryResults map[prom.Selector]prom.QueryResult
}

func (c *FakePrometheusClient) Series(_ context.Context, interval pmodel.Interval, selectors ...prom.Selector) ([]prom.Series, error) {
	if (interval.Start != 0 && interval.Start < c.AcceptableInterval.Start) || (interval.End != 0 && interval.End > c.AcceptableInterval.End) {
		return nil, fmt.Errorf("interval [%v, %v] for query is outside range [%v, %v]", interval.Start, interval.End, c.AcceptableInterval.Start, c.AcceptableInterval.End)
	}
	res := []prom.Series{}
	for _, sel := range selectors {
		if err, found := c.ErrQueries[sel]; found {
			return nil, err
		}
		if series, found := c.SeriesResults[sel]; found {
			res = append(res, series...)
		}
	}

	return res, nil
}

func (c *FakePrometheusClient) Query(_ context.Context, t pmodel.Time, query prom.Selector) (prom.QueryResult, error) {
	if t < c.AcceptableInterval.Start || t > c.AcceptableInterval.End {
		return prom.QueryResult{}, fmt.Errorf("time %v for query is outside range [%v, %v]", t, c.AcceptableInterval.Start, c.AcceptableInterval.End)
	}

	if err, found := c.ErrQueries[query]; found {
		return prom.QueryResult{}, err
	}

	if res, found := c.QueryResults[query]; found {
		return res, nil
	}

	return prom.QueryResult{
		Type:   pmodel.ValVector,
		Vector: &pmodel.Vector{},
	}, nil
}

func (c *FakePrometheusClient) QueryRange(_ context.Context, r prom.Range, query prom.Selector) (prom.QueryResult, error) {
	return prom.QueryResult{}, nil
}
