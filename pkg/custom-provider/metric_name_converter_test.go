package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	config "github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

func TestWhenNoMappingMetricNameIsUnaltered(t *testing.T) {
	emptyMapping := config.NameMapping{}
	RunTest(t, emptyMapping, "my_series", "my_series")
	RunTest(t, emptyMapping, "your_series", "your_series")
}

func TestWhenMappingWithOneCaptureGroupMetricNameIsCorrect(t *testing.T) {
	mapping := config.NameMapping{
		Matches: "my_(.*)",
		As:      "your_$1",
	}

	RunTest(t, mapping, "my_requests_per_second", "your_requests_per_second")
}

func TestWhenMappingWithMultipleCaptureGroupsMetricNameIsCorrect(t *testing.T) {
	//ExpandString has some strange behavior when using the $1, $2 syntax
	//Specifically, it doesn't return the expected values for templates like:
	//$1_$2
	//You can work around it by using the ${1} syntax.
	mapping := config.NameMapping{
		Matches: "my_([^_]+)_([^_]+)",
		As:      "your_${1}_is_${2}_large",
	}

	RunTest(t, mapping, "my_horse_very", "your_horse_is_very_large")
	RunTest(t, mapping, "my_dog_not", "your_dog_is_not_large")
}

func TestAsCanBeInferred(t *testing.T) {
	//When we've got one capture group, we should infer that as the target.
	mapping := config.NameMapping{
		Matches: "my_(.+)",
	}

	RunTest(t, mapping, "my_test_metric", "test_metric")

	//When we have no capture groups, we should infer that the whole thing as the target.
	mapping = config.NameMapping{
		Matches: "my_metric",
	}

	RunTest(t, mapping, "my_metric", "my_metric")
}

func TestWhenAsCannotBeInferredError(t *testing.T) {
	//More than one capture group should
	//result in us giving up on making an educated guess.
	mapping := config.NameMapping{
		Matches: "my_([^_]+)_([^_]+)",
	}

	RunTestExpectingError(t, mapping, "my_horse_very")
	RunTestExpectingError(t, mapping, "my_dog_not")
}

func RunTest(t *testing.T, mapping config.NameMapping, input string, expectedResult string) {
	converter, err := NewMetricNameConverter(mapping)
	require.NoError(t, err)

	series := prom.Series{
		Name: input,
	}

	actualResult, err := converter.GetMetricNameForSeries(series)
	require.NoError(t, err)

	require.Equal(t, expectedResult, actualResult)
}

func RunTestExpectingError(t *testing.T, mapping config.NameMapping, input string) {
	_, err := NewMetricNameConverter(mapping)
	require.Error(t, err)
}
