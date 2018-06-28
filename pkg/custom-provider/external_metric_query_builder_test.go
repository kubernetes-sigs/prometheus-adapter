package provider

import (
	"testing"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var queryBuilder = NewExternalMetricQueryBuilder()

func TestBuildPrometheusQuery(t *testing.T) {
	fakeSelector := labels.NewSelector()
	requirement, _ := labels.NewRequirement("queue_name", selection.Equals, []string{"processing"})
	fakeSelector = fakeSelector.Add(*requirement)

	result := queryBuilder.BuildPrometheusQuery("default", "queue_length", fakeSelector)

	expectedResult := "queue_length{namespace=\"default\", queue_name=\"processing\"}"
	if result != expectedResult {
		t.Errorf("Incorrect query generated. Expected: %s | Actual %s", result, expectedResult)
	}
}
