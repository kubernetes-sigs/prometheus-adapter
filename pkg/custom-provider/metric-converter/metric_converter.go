package provider

import (
	"errors"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

//MetricConverter provides a unified interface for converting the results of
//Prometheus queries into external metric types.
type MetricConverter interface {
	Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error)
}

type metricConverter struct {
	scalarConverter MetricConverter
	vectorConverter MetricConverter
	matrixConverter MetricConverter
}

//NewMetricConverter creates a MetricCoverter, capable of converting any of the three metric types
//returned by the Prometheus client into external metrics types.
func NewMetricConverter(scalar MetricConverter, vector MetricConverter, matrix MetricConverter) MetricConverter {
	return &metricConverter{
		scalarConverter: scalar,
		vectorConverter: vector,
		matrixConverter: matrix,
	}
}

func (c *metricConverter) Convert(queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type == model.ValScalar {
		return c.scalarConverter.Convert(queryResult)
	}

	if queryResult.Type == model.ValVector {
		return c.vectorConverter.Convert(queryResult)
	}

	if queryResult.Type == model.ValMatrix {
		return c.matrixConverter.Convert(queryResult)
	}

	return nil, errors.New("encountered an unexpected query result type")
}
