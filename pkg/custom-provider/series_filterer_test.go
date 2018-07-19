package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

func TestPositiveFilter(t *testing.T) {
	filters := []config.RegexFilter{
		config.RegexFilter{
			Is: "one_of_(yes|positive|ok)",
		},
	}

	series := []string{"one_of_yes", "one_of_no"}
	expectedSeries := []string{"one_of_yes"}

	RunSeriesFiltererTest(t, filters, series, expectedSeries)
}

func TestNegativeFilter(t *testing.T) {
	filters := []config.RegexFilter{
		config.RegexFilter{
			IsNot: "one_of_(yes|positive|ok)",
		},
	}

	series := []string{"one_of_yes", "one_of_no"}
	expectedSeries := []string{"one_of_no"}

	RunSeriesFiltererTest(t, filters, series, expectedSeries)
}

func TestPositiveAndNegativeFilterError(t *testing.T) {
	filters := []config.RegexFilter{
		config.RegexFilter{
			Is:    "series_\\d+",
			IsNot: "series_[2-3]+",
		},
	}

	_, err := NewSeriesFilterer(filters)
	require.Error(t, err)
}

func TestAddRequirementAppliesFilter(t *testing.T) {
	seriesNames := []string{"series_1", "series_2", "series_3"}
	series := BuildSeriesFromNames(seriesNames)

	filters := []config.RegexFilter{
		config.RegexFilter{
			Is: "series_\\d+",
		},
	}

	filterer, err := NewSeriesFilterer(filters)
	require.NoError(t, err)

	//Test it once with the default filters.
	result := filterer.FilterSeries(series)
	expectedSeries := []string{"series_1", "series_2", "series_3"}
	VerifyMatches(t, result, expectedSeries)

	//Add a new filter and test again.
	filterer.AddRequirement(config.RegexFilter{
		Is: "series_[2-3]",
	})
	result = filterer.FilterSeries(series)
	expectedSeries = []string{"series_2", "series_3"}
	VerifyMatches(t, result, expectedSeries)
}

func RunSeriesFiltererTest(t *testing.T, filters []config.RegexFilter, seriesNames []string, expectedResults []string) {
	series := BuildSeriesFromNames(seriesNames)

	filterer, err := NewSeriesFilterer(filters)
	require.NoError(t, err)

	matches := filterer.FilterSeries(series)

	VerifyMatches(t, matches, expectedResults)
}

func VerifyMatches(t *testing.T, series []prom.Series, expectedResults []string) {
	require.Equal(t, len(series), len(expectedResults))

	existingSeries := make(map[string]bool)

	for _, series := range series {
		existingSeries[series.Name] = true
	}

	for _, expectedResult := range expectedResults {
		_, exists := existingSeries[expectedResult]

		require.True(t, exists)
	}
}

func BuildSeriesFromNames(seriesNames []string) []prom.Series {
	series := make([]prom.Series, len(seriesNames))

	for i, name := range seriesNames {
		series[i] = prom.Series{
			Name: name,
		}
	}

	return series
}
