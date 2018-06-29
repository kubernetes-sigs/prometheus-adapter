package provider

import (
	"errors"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type vectorConverter struct {
}

//NewVectorConverter creates a VectorConverter capable of converting
//vector Prometheus query results into external metric types.
func NewVectorConverter() MetricConverter {
	return &vectorConverter{}
}

func (c *vectorConverter) Convert(metadata QueryMetadata, queryResult prom.QueryResult) (*external_metrics.ExternalMetricValueList, error) {
	if queryResult.Type != model.ValVector {
		return nil, errors.New("vectorConverter can only convert scalar query results")
	}

	toConvert := *queryResult.Vector

	if toConvert == nil {
		return nil, errors.New("the provided input did not contain vector query results")
	}

	return c.convert(metadata, toConvert)
}

func (c *vectorConverter) convert(metadata QueryMetadata, result model.Vector) (*external_metrics.ExternalMetricValueList, error) {
	items := []external_metrics.ExternalMetricValue{}
	metricValueList := external_metrics.ExternalMetricValueList{
		Items: items,
	}

	numSamples := result.Len()
	if numSamples == 0 {
		return &metricValueList, nil
	}

	for _, val := range result {
		singleMetric := external_metrics.ExternalMetricValue{
			MetricName: string(val.Metric[model.LabelName("__name__")]),
			Timestamp: metav1.Time{
				val.Timestamp.Time(),
			},
			WindowSeconds: &metadata.WindowInSeconds,
			//TODO: I'm not so sure about this type/conversions.
			//This can't possibly be the right way to convert this.
			//Also, does K8S only deal win integer metrics?
			Value: *resource.NewQuantity(int64(float64(val.Value)), resource.DecimalSI),
		}

		items = append(items, singleMetric)
	}

	return &metricValueList, nil
}
