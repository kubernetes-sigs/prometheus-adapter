package provider

import (
	"errors"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type matrixConverter struct {
}

//NewMatrixConverter creates a MatrixConverter capable of converting
//matrix Prometheus query results into external metric types.
func NewMatrixConverter() MetricConverter {
	return &matrixConverter{}
}

func (c *matrixConverter) Convert(metadata QueryMetadata, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValMatrix {
		return nil, errors.New("matrixConverter can only convert scalar query results")
	}

	toConvert := queryResult.Matrix

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain matrix query results")
	}

	return c.convert(toConvert)
}

func (c *matrixConverter) convert(result *model.Matrix) (*external_metrics.ExternalMetricValueList, error) {
	//TODO: Implementation.
	return nil, errors.New("converting Matrix results is not yet supported")
}
