package provider

import (
	"testing"

	"k8s.io/apimachinery/pkg/selection"

	"github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/stretchr/testify/require"
)

func TestBadQueryPartsDontBuildQueries(t *testing.T) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")
	_, err := builder.BuildSelector("my_series", "", []string{}, []queryPart{
		queryPart{
			labelName: "",
			values:    nil,
		},
		queryPart{
			labelName: "",
			values:    []string{},
		},
	})

	require.Error(t, err)
}

func runQueryBuilderTest(t *testing.T, queryParts []queryPart, expectation string) {
	builder, _ := NewQueryBuilder("rate(<<.Series>>{<<.LabelMatchers>>}[2m])")
	selector, err := builder.BuildSelector("my_series", "", []string{}, queryParts)

	expectError := expectation == ""

	if expectError {
		require.Error(t, err)
	} else {
		selectorExpectation := client.Selector(expectation)
		require.NoError(t, err)
		require.Equal(t, selector, selectorExpectation)
	}
}

func TestSimpleQuery(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{}, "")
}

//Equals
func TestEqualsQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.Equals,
		},
	}, "")
}

func TestEqualsQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.Equals,
		},
	}, "rate(my_series{target_label=\"one\"}[2m])")
}

func TestEqualsQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.Equals,
		},
	}, "rate(my_series{target_label=~\"one|two\"}[2m])")
}

//Double Equals
func TestDoubleEqualsQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.DoubleEquals,
		},
	}, "")
}

func TestDoubleEqualsQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.DoubleEquals,
		},
	}, "rate(my_series{target_label=\"one\"}[2m])")
}

func TestDoubleEqualsQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.DoubleEquals,
		},
	}, "rate(my_series{target_label=~\"one|two\"}[2m])")
}

//Not Equals
func TestNotEqualsQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.NotEquals,
		},
	}, "")
}

func TestNotEqualsQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.NotEquals,
		},
	}, "rate(my_series{target_label!=\"one\"}[2m])")
}

func TestNotEqualsQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.NotEquals,
		},
	}, "rate(my_series{target_label!~\"one|two\"}[2m])")
}

//In
func TestInQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.In,
		},
	}, "")
}

func TestInQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.In,
		},
	}, "rate(my_series{target_label=~\"one\"}[2m])")
}

func TestInQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.In,
		},
	}, "rate(my_series{target_label=~\"one|two\"}[2m])")
}

//NotIn
func TestNotInQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.NotIn,
		},
	}, "")
}

func TestNotInQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.NotIn,
		},
	}, "rate(my_series{target_label!~\"one\"}[2m])")
}

func TestNotInQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.NotIn,
		},
	}, "rate(my_series{target_label!~\"one|two\"}[2m])")
}

//Exists
func TestExistsQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.Exists,
		},
	}, "rate(my_series{target_label=~\".+\"}[2m])")
}

func TestExistsQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.Exists,
		},
	}, "")
}

func TestExistsQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.Exists,
		},
	}, "")
}

//DoesNotExist
func TestDoesNotExistsQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.DoesNotExist,
		},
	}, "rate(my_series{target_label!~\".+\"}[2m])")
}

func TestDoesNotExistsQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.DoesNotExist,
		},
	}, "")
}

func TestDoesNotExistsQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.DoesNotExist,
		},
	}, "")
}

//GreaterThan
func TestGreaterThanQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.GreaterThan,
		},
	}, "")
}

func TestGreaterThanQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.GreaterThan,
		},
	}, "")
}

func TestGreaterThanQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.GreaterThan,
		},
	}, "")
}

//LessThan
func TestLessThanQueryWithNoLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{},
			operator:  selection.LessThan,
		},
	}, "")
}

func TestLessThanQueryWithOneLabelValue(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one"},
			operator:  selection.LessThan,
		},
	}, "")
}

func TestLessThanQueryWithMultipleLabelValues(t *testing.T) {
	runQueryBuilderTest(t, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.LessThan,
		},
	}, "")
}

func TestQueryWithGroupBy(t *testing.T) {
	builder, _ := NewQueryBuilder("sum(rate(<<.Series>>{<<.LabelMatchers>>}[2m])) by (<<.GroupBy>>)")

	selector, _ := builder.BuildSelector("my_series", "my_grouping", []string{}, []queryPart{
		queryPart{
			labelName: "target_label",
			values:    []string{"one", "two"},
			operator:  selection.In,
		},
	})

	expectation := client.Selector("sum(rate(my_series{target_label=~\"one|two\"}[2m])) by (my_grouping)")
	require.Equal(t, selector, expectation)
}

// TODO: Ensure that the LabelValuesByName and GroupBySlice placeholders function correctly.
