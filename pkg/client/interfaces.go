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
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

// NB: the official prometheus API client at https://github.com/prometheus/client_golang
// is rather lackluster -- as of the time of writing of this file, it lacked support
// for querying the series metadata, which we need for the adapter. Instead, we use
// this client.

// Selector represents a series selector
type Selector string

// Range represents a sliced time range with increments.
type Range struct {
	// Start and End are the boundaries of the time range.
	Start, End model.Time
	// Step is the maximum time between two slices within the boundaries.
	Step time.Duration
}

// Client is a Prometheus client for the Prometheus HTTP API.
// The "timeout" parameter for the HTTP API is set based on the context's deadline,
// when present and applicable.
type Client interface {
	// Series lists the time series matching the given series selectors
	Series(ctx context.Context, interval model.Interval, selectors ...Selector) ([]Series, error)
	// Query runs a non-range query at the given time.
	Query(ctx context.Context, t model.Time, query Selector) (QueryResult, error)
	// QueryRange runs a range query at the given time.
	QueryRange(ctx context.Context, r Range, query Selector) (QueryResult, error)
}

// QueryResult is the result of a query.
// Type will always be set, as well as one of the other fields, matching the type.
type QueryResult struct {
	Type model.ValueType

	Vector *model.Vector
	Scalar *model.Scalar
	Matrix *model.Matrix
}

func (qr *QueryResult) UnmarshalJSON(b []byte) error {
	v := struct {
		Type   model.ValueType `json:"resultType"`
		Result json.RawMessage `json:"result"`
	}{}

	err := json.Unmarshal(b, &v)
	if err != nil {
		return err
	}

	qr.Type = v.Type

	switch v.Type {
	case model.ValScalar:
		var sv model.Scalar
		err = json.Unmarshal(v.Result, &sv)
		qr.Scalar = &sv

	case model.ValVector:
		var vv model.Vector
		err = json.Unmarshal(v.Result, &vv)
		qr.Vector = &vv

	case model.ValMatrix:
		var mv model.Matrix
		err = json.Unmarshal(v.Result, &mv)
		qr.Matrix = &mv

	default:
		err = fmt.Errorf("unexpected value type %q", v.Type)
	}
	return err
}

// Series represents a description of a series: a name and a set of labels.
// Series is roughly equivalent to model.Metrics, but has easy access to name
// and the set of non-name labels.
type Series struct {
	Name   string
	Labels model.LabelSet
}

func (s *Series) UnmarshalJSON(data []byte) error {
	var rawMetric model.Metric
	err := json.Unmarshal(data, &rawMetric)
	if err != nil {
		return err
	}

	if name, ok := rawMetric[model.MetricNameLabel]; ok {
		s.Name = string(name)
		delete(rawMetric, model.MetricNameLabel)
	}

	s.Labels = model.LabelSet(rawMetric)

	return nil
}

func (s *Series) String() string {
	lblStrings := make([]string, 0, len(s.Labels))
	for k, v := range s.Labels {
		lblStrings = append(lblStrings, fmt.Sprintf("%s=%q", k, v))
	}
	return fmt.Sprintf("%s{%s}", s.Name, strings.Join(lblStrings, ","))
}
