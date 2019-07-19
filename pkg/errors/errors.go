package errors

import (
	"github.com/directxman12/k8s-prometheus-adapter/pkg/metrics"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewMetricNotFoundError(resource schema.GroupResource, metricName string) error {
	metrics.Errors.WithLabelValues("not_found").Inc()
	return provider.NewMetricNotFoundError(resource, metricName)
}

func NewInternalError(err error) *apierr.StatusError {
	metrics.Errors.WithLabelValues("internal").Inc()
	return apierr.NewInternalError(err)
}
