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
	"testing"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	pmodel "github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type resourceConverterMock struct {
	namespaced bool
}

// ResourcesForSeries is a mock that returns a single group resource,
// namely the series as a resource itself.
func (rcm *resourceConverterMock) ResourcesForSeries(series prom.Series) (res []schema.GroupResource, namespaced bool) {
	return []schema.GroupResource{{Resource: series.Name}}, false
}

// LabelForResource is a mock that returns the label name,
// simply by taking the given resource.
func (rcm *resourceConverterMock) LabelForResource(gr schema.GroupResource) (pmodel.LabelName, error) {
	return pmodel.LabelName(gr.Resource), nil
}

func TestBuildSelector(t *testing.T) {
	type checkFunc func(prom.Selector, error) error

	hasError := func(want error) checkFunc {
		return func(_ prom.Selector, got error) error {
			if want != got {
				return fmt.Errorf("got error %v, want %v", got, want)
			}
			return nil
		}
	}

	hasSelector := func(want string) checkFunc {
		return func(got prom.Selector, _ error) error {
			if prom.Selector(want) != got {
				return fmt.Errorf("got selector %q, want %q", got, want)
			}
			return nil
		}
	}

	checks := func(cs ...checkFunc) checkFunc {
		return func(s prom.Selector, e error) error {
			for _, c := range cs {
				if err := c(s, e); err != nil {
					return err
				}
			}
			return nil
		}
	}

	mustNewQuery := func(queryTemplate string, namespaced bool) MetricsQuery {
		mq, err := NewMetricsQuery(queryTemplate, &resourceConverterMock{namespaced})
		if err != nil {
			t.Fatal(err)
		}
		return mq
	}

	tests := []struct {
		name string
		mq   MetricsQuery

		series       string
		resource     schema.GroupResource
		namespace    string
		extraGroupBy []string
		names        []string

		check checkFunc
	}{
		{
			name: "series",

			mq:     mustNewQuery(`series <<.Series>>`, false),
			series: "foo",

			check: checks(
				hasError(nil),
				hasSelector("series foo"),
			),
		},

		{
			name: "multiple LabelMatchers values",

			mq:       mustNewQuery(`<<.LabelMatchers>>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},
			names:    []string{"bar", "baz"},

			check: checks(
				hasError(nil),
				hasSelector(`resource=~"bar|baz"`),
			),
		},

		{
			name: "single LabelMatchers value",

			mq:       mustNewQuery(`<<.LabelMatchers>>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},
			names:    []string{"bar"},

			check: checks(
				hasError(nil),
				hasSelector(`resource="bar"`),
			),
		},

		{
			name: "single LabelValuesByName value",

			mq:       mustNewQuery(`<<index .LabelValuesByName "resource">>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},
			names:    []string{"bar"},

			check: checks(
				hasError(nil),
				hasSelector("bar"),
			),
		},

		{
			name: "multiple LabelValuesByName values",

			mq:       mustNewQuery(`<<index .LabelValuesByName "resource">>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},
			names:    []string{"bar", "baz"},

			check: checks(
				hasError(nil),
				hasSelector("bar|baz"),
			),
		},

		{
			name: "multiple LabelValuesByName values with namespace",

			mq:        mustNewQuery(`<<index .LabelValuesByName "namespaces">> <<index .LabelValuesByName "resource">>`, true),
			resource:  schema.GroupResource{Group: "group", Resource: "resource"},
			namespace: "default",
			names:     []string{"bar", "baz"},

			check: checks(
				hasError(nil),
				hasSelector("default bar|baz"),
			),
		},

		{
			name: "single GroupBy value",

			mq:       mustNewQuery(`<<.GroupBy>>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},

			check: checks(
				hasError(nil),
				hasSelector("resource"),
			),
		},

		{
			name: "multiple GroupBySlice values",

			mq:           mustNewQuery(`<<.GroupBy>>`, false),
			resource:     schema.GroupResource{Group: "group", Resource: "resource"},
			extraGroupBy: []string{"extra", "groups"},

			check: checks(
				hasError(nil),
				hasSelector("resource,extra,groups"),
			),
		},

		{
			name: "single GroupBySlice value",

			mq:       mustNewQuery(`<<.GroupBySlice>>`, false),
			resource: schema.GroupResource{Group: "group", Resource: "resource"},

			check: checks(
				hasError(nil),
				hasSelector("[resource]"),
			),
		},

		{
			name: "multiple GroupBy values",

			mq:           mustNewQuery(`<<.GroupBySlice>>`, false),
			resource:     schema.GroupResource{Group: "group", Resource: "resource"},
			extraGroupBy: []string{"extra", "groups"},

			check: checks(
				hasError(nil),
				hasSelector("[resource extra groups]"),
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			selector, err := tc.mq.Build(tc.series, tc.resource, tc.namespace, tc.extraGroupBy, tc.names...)

			if err := tc.check(selector, err); err != nil {
				t.Error(err)
			}
		})
	}
}
