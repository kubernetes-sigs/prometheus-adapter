package provider

import (
	"errors"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type vectorConverter struct {
}

//NewVectorConverter creates a VectorConverter capable of converting
//vector Prometheus query results into external metric types.
func NewVectorConverter() MetricConverter {
	return &vectorConverter{}
}

func (c *vectorConverter) Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValVector {
		return nil, errors.New("vectorConverter can only convert scalar query results")
	}

	toConvert := queryResult.Vector

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain vector query results")
	}

	return c.convert(toConvert)
}

func (c *vectorConverter) convert(result *model.Vector) (*external_metrics.ExternalMetricValueList, error) {
	//TODO: Implementation.
	return nil, nil
}
