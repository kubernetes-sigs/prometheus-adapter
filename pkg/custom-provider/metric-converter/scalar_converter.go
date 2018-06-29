package provider

import (
	"errors"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type scalarConverter struct {
}

//NewScalarConverter creates a ScalarConverter capable of converting
//scalar Prometheus query results into external metric types.
func NewScalarConverter() MetricConverter {
	return &scalarConverter{}
}

func (c *scalarConverter) Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValScalar {
		return nil, errors.New("scalarConverter can only convert scalar query results")
	}

	toConvert := queryResult.Scalar

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain scalar query results")
	}

	return c.convert(toConvert)
}

func (c *scalarConverter) convert(input *model.Scalar) (*external_metrics.ExternalMetricValueList, error) {
	tempWindow := int64(0)
	result := external_metrics.ExternalMetricValueList{
		//TODO: Where should all of these values come from?
		TypeMeta: metav1.TypeMeta{
			Kind:       "?",
			APIVersion: "?",
		},
		ListMeta: metav1.ListMeta{
			SelfLink:        "?",
			ResourceVersion: "?",
			Continue:        "?",
		},
		Items: []external_metrics.ExternalMetricValue{
			external_metrics.ExternalMetricValue{
				TypeMeta: metav1.TypeMeta{
					Kind:       "?",
					APIVersion: "?",
				},
				//TODO: Carry forward the metric name so we can set it here.
				MetricName: "?",
				Timestamp: metav1.Time{
					input.Timestamp.Time(),
				},
				//TODO: Carry forward some information about our configuration so we can provide it here.
				WindowSeconds: &tempWindow,
				//TODO: Jump through the necessary hoops to convert our number into the proper type.
				Value: resource.Quantity{},
			},
		},
	}
	return &result, nil
}
