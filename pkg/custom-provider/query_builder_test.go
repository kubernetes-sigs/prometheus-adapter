package provider

import (
	"testing"

	"github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/stretchr/testify/require"
)

func TestBadQueryPartsDontError(t *testing.T) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")
	selector, err := builder.BuildSelector("my_series", "", []string{}, []queryPart{
		queryPart{
			labelName: "",
			values:    nil,
		},
		queryPart{
			labelName: "",
			values:    []string{},
		},
	})

	expectation := client.Selector("rate(my_series{}[2m])")
	require.NoError(t, err)
	require.Equal(t, selector, expectation)
}

func TestSimpleQuery(t *testing.T) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")

	// builder, _ := NewQueryBuilder("sum(rate(<<.Series>>{<<.LabelMatchers>>,static_label!=\"static_value\"}[2m])) by (<<.GroupBy>>)")
	selector, _ := builder.BuildSelector("my_series", "", []string{}, []queryPart{})

	expectation := client.Selector("rate(my_series{}[2m])")
	require.Equal(t, selector, expectation)
}

func TestSimpleQueryWithOneLabelValue(t *testing.T) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")

	// builder, _ := NewQueryBuilder("sum(rate(<<.Series>>{<<.LabelMatchers>>,static_label!=\"static_value\"}[2m])) by (<<.GroupBy>>)")
	selector, _ := builder.BuildSelector("my_series", "", []string{}, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
		},
	})

	expectation := client.Selector("rate(my_series{target_label=\"one\"}[2m])")
	require.Equal(t, selector, expectation)
}

func TestSimpleQueryWithMultipleLabelValues(t *testing.T) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")

	// builder, _ := NewQueryBuilder("sum(rate(<<.Series>>{<<.LabelMatchers>>,static_label!=\"static_value\"}[2m])) by (<<.GroupBy>>)")
	selector, _ := builder.BuildSelector("my_series", "", []string{}, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
		},
	})

	expectation := client.Selector("rate(my_series{target_label=~\"one|two\"}[2m])")
	require.Equal(t, selector, expectation)
}

func TestQueryWithGroupBy(t *testing.T) {
	builder, _ := NewQueryBuilder("sum(rate(<<.Series>>{<<.LabelMatchers>>}[2m])) by (<<.GroupBy>>)")

	selector, _ := builder.BuildSelector("my_series", "my_grouping", []string{}, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
		},
	})

	expectation := client.Selector("sum(rate(my_series{target_label=~\"one|two\"}[2m])) by (my_grouping)")
	require.Equal(t, selector, expectation)
}

//TODO: AC - Ensure that the LabelValuesByName and GroupBySlice placeholders function correctly.
