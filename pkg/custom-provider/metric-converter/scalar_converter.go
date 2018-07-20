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
	result := external_metrics.ExternalMetricValueList{
		//Using prometheusProvider.metricsFor(...) as an example,
		//it seems that I don't need to provide values for
		//TypeMeta and ListMeta.
		//TODO: Get some confirmation on this.
		Items: []external_metrics.ExternalMetricValue{
			{
				Timestamp: metav1.Time{
					input.Timestamp.Time(),
				},
				//TODO: I'm not so sure about this type/conversions.
				//Is there a meaningful loss of precision here?
				//Does K8S only deal win integer metrics?
				Value: *resource.NewMilliQuantity(int64(input.Value*1000.0), resource.DecimalSI),
			},
		},
	}
	return &result, nil
}
