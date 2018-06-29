package provider

import (
	"testing"

	conv "github.com/directxman12/k8s-prometheus-adapter/pkg/custom-provider/metric-converter"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var queryBuilder = NewExternalMetricQueryBuilder()

func TestBuildPrometheusQuery(t *testing.T) {
	fakeSelector := labels.NewSelector()
	metricName := "queue_name"
	requirement, _ := labels.NewRequirement(metricName, selection.Equals, []string{"processing"})
	fakeSelector = fakeSelector.Add(*requirement)
	meta := conv.QueryMetadata{
		Aggregation:     "rate",
		MetricName:      metricName,
		WindowInSeconds: 120,
	}

	result := queryBuilder.BuildPrometheusQuery("default", "queue_length", fakeSelector, meta)

	expectedResult := "rate(queue_length{queue_name=\"processing\"}[120s])"
	if result != expectedResult {
		t.Errorf("Incorrect query generated. Expected: %s | Actual %s", result, expectedResult)
	}
}
