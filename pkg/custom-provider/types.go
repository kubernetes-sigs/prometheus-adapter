package provider

import (
	"context"
	"fmt"
	"time"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/golang/glog"
	pmodel "github.com/prometheus/common/model"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

type cachingMetricsLister struct {
	SeriesRegistry

	promClient     prom.Client
	updateInterval time.Duration
}

func (l *cachingMetricsLister) Run() {
	l.RunUntil(wait.NeverStop)
}

func (l *cachingMetricsLister) RunUntil(stopChan <-chan struct{}) {
	go wait.Until(func() {
		if err := l.updateMetrics(); err != nil {
			utilruntime.HandleError(err)
		}
	}, l.updateInterval, stopChan)
}

func (l *cachingMetricsLister) updateMetrics() error {
	startTime := pmodel.Now().Add(-1 * l.updateInterval)

	sels := l.Selectors()

	// TODO: use an actual context here
	series, err := l.promClient.Series(context.Background(), pmodel.Interval{startTime, 0}, sels...)
	if err != nil {
		return fmt.Errorf("unable to update list of all available metrics: %v", err)
	}

	glog.V(10).Infof("Set available metric list from Prometheus to: %v", series)

	l.SetSeries(series)

	return nil
}
