package provider

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/golang/glog"
	pmodel "github.com/prometheus/common/model"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// TODO: Make sure everything has the proper licensing disclosure at the top.
type externalPrometheusProvider struct {
	promClient      prom.Client
	metricConverter MetricConverter

	seriesRegistry ExternalSeriesRegistry
}

// NewExternalPrometheusProvider creates an ExternalMetricsProvider capable of responding to Kubernetes requests for external metric data.
func NewExternalPrometheusProvider(seriesRegistry ExternalSeriesRegistry, promClient prom.Client, converter MetricConverter) provider.ExternalMetricsProvider {
	return &externalPrometheusProvider{
		promClient:      promClient,
		seriesRegistry:  seriesRegistry,
		metricConverter: converter,
	}
}

func (p *externalPrometheusProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	selector, found, err := p.seriesRegistry.QueryForMetric(namespace, metricName, metricSelector)

	if err != nil {
		glog.Errorf("unable to generate a query for the metric: %v", err)
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	if !found {
		return nil, provider.NewMetricNotFoundError(p.selectGroupResource(namespace), metricName)
	}

	queryResults, err := p.promClient.Query(context.TODO(), pmodel.Now(), selector)

	if err != nil {
		glog.Errorf("unable to fetch metrics from prometheus: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to fetch metrics"))
	}

	return p.metricConverter.Convert(queryResults)
}

func (p *externalPrometheusProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	return p.seriesRegistry.ListAllMetrics()
}

func (p *externalPrometheusProvider) selectGroupResource(namespace string) schema.GroupResource {
	if namespace == "" {
		return nsGroupResource
	}

	return schema.GroupResource{
		Group:    "",
		Resource: "",
	}
}
