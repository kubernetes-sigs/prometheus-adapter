package provider

import (
	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type sampleConverter struct {
}

//SampleConverter is capable of translating Prometheus Sample objects
//into ExternamMetricValue objects.
type SampleConverter interface {
	Convert(sample *model.Sample) (*external_metrics.ExternalMetricValue, error)
}

//NewSampleConverter creates a SampleConverter capable of translating Prometheus Sample objects
//into ExternamMetricValue objects.
func NewSampleConverter() SampleConverter {
	return &sampleConverter{}
}

func (c *sampleConverter) Convert(sample *model.Sample) (*external_metrics.ExternalMetricValue, error) {
	labels := c.convertLabels(sample.Metric)

	singleMetric := external_metrics.ExternalMetricValue{
		MetricName: string(sample.Metric[model.LabelName("__name__")]),
		Timestamp: metav1.Time{
			sample.Timestamp.Time(),
		},
		//TODO: I'm not so sure about this type/conversions.
		//This can't possibly be the right way to convert this.
		//Also, does K8S only deal win integer metrics?
		Value:        *resource.NewQuantity(int64(float64(sample.Value)), resource.DecimalSI),
		MetricLabels: labels,
	}

	//TODO: Actual errors?
	return &singleMetric, nil
}

func (c *sampleConverter) convertLabels(inLabels model.Metric) map[string]string {
	numLabels := len(inLabels)
	outLabels := make(map[string]string, numLabels)
	for labelName, labelVal := range inLabels {
		outLabels[string(labelName)] = string(labelVal)
	}

	return outLabels
}
