package provider

import (
	"context"

	pmodel "github.com/prometheus/common/model"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"

	conv "github.com/directxman12/k8s-prometheus-adapter/pkg/custom-provider/metric-converter"
)

//TODO: AC - Make sure everything has the proper licensing disclosure at the top.
type externalPrometheusProvider struct {
	promClient      prom.Client
	metricConverter conv.MetricConverter

	seriesRegistry ExternalSeriesRegistry
}

//NewExternalPrometheusProvider creates an ExternalMetricsProvider capable of responding to Kubernetes requests for external metric data.
func NewExternalPrometheusProvider(seriesRegistry ExternalSeriesRegistry, promClient prom.Client, converter conv.MetricConverter) provider.ExternalMetricsProvider {
	return &externalPrometheusProvider{
		promClient:      promClient,
		seriesRegistry:  seriesRegistry,
		metricConverter: converter,
	}
}

func (p *externalPrometheusProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	selector, found := p.seriesRegistry.QueryForMetric(namespace, metricName, metricSelector)

	if !found {
		return &external_metrics.ExternalMetricValueList{
			Items: []external_metrics.ExternalMetricValue{},
		}, nil
	}

	queryResults, err := p.promClient.Query(context.TODO(), pmodel.Now(), selector)

	if err != nil {
		return nil, err
	}

	return p.metricConverter.Convert(queryResults)
}

func (p *externalPrometheusProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	return p.seriesRegistry.ListAllMetrics()
}
