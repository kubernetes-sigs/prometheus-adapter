/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	pmodel "github.com/prometheus/common/model"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// Runnable represents something that can be run until told to stop.
type Runnable interface {
	// Run runs the runnable forever.
	Run()
	// RunUntil runs the runnable until the given channel is closed.
	RunUntil(stopChan <-chan struct{})
}

//A MetricLister provides a window into all of the metrics that are available within a given
//Prometheus instance, classified as either Custom or External metrics, but presented generically
//so that it can manage both types simultaneously.
type MetricLister interface {
	// Run()
	// UpdateMetrics() error
	// GetAllMetrics() []GenericMetricInfo
	// GetAllCustomMetrics() []GenericMetricInfo
	// GetAllExternalMetrics() []GenericMetricInfo
	// GetInfoForMetric(infoKey GenericMetricInfo) (seriesInfo, bool)
	ListAllMetrics() (metricUpdateResult, error)
}

type MetricListerWithNotification interface {
	//It can list metrics, just like a normal MetricLister.
	MetricLister
	//Because it periodically pulls metrics, it needs to be Runnable.
	Runnable
	//It provides notifications when it has new data to supply.
	AddNotificationReceiver(func(metricUpdateResult))
	UpdateNow()
}

type basicMetricLister struct {
	promClient prom.Client
	namers     []MetricNamer
	lookback   time.Duration
}

func NewBasicMetricLister(promClient prom.Client, namers []MetricNamer, lookback time.Duration) MetricLister {
	lister := basicMetricLister{
		promClient: promClient,
		namers:     namers,
		lookback:   lookback,
	}

	return &lister
}

type selectorSeries struct {
	selector prom.Selector
	series   []prom.Series
}

func (l *basicMetricLister) ListAllMetrics() (metricUpdateResult, error) {
	result := metricUpdateResult{
		series: make([][]prom.Series, 0),
		namers: make([]MetricNamer, 0),
	}

	startTime := pmodel.Now().Add(-1 * l.lookback)

	// these can take a while on large clusters, so launch in parallel
	// and don't duplicate
	selectors := make(map[prom.Selector]struct{})
	selectorSeriesChan := make(chan selectorSeries, len(l.namers))
	errs := make(chan error, len(l.namers))
	for _, namer := range l.namers {
		sel := namer.Selector()
		if _, ok := selectors[sel]; ok {
			errs <- nil
			selectorSeriesChan <- selectorSeries{}
			continue
		}
		selectors[sel] = struct{}{}
		go func() {
			series, err := l.promClient.Series(context.TODO(), pmodel.Interval{startTime, 0}, sel)
			if err != nil {
				errs <- fmt.Errorf("unable to fetch metrics for query %q: %v", sel, err)
				return
			}
			errs <- nil
			//Push into the channel: "this selector produced these series"
			selectorSeriesChan <- selectorSeries{
				selector: sel,
				series:   series,
			}
		}()
	}

	// don't do duplicate queries when it's just the matchers that change
	seriesCacheByQuery := make(map[prom.Selector][]prom.Series)

	// iterate through, blocking until we've got all results
	// We know that, from above, we should have pushed one item into the channel
	// for each namer. So here, we'll assume that we should receive one item per namer.
	for range l.namers {
		if err := <-errs; err != nil {
			return result, fmt.Errorf("unable to update list of all metrics: %v", err)
		}
		//Receive from the channel: "this selector produced these series"
		//We stuff that into this map so that we can collect the data as it arrives
		//and then, once we've received it all, we can process it below.
		if ss := <-selectorSeriesChan; ss.series != nil {
			seriesCacheByQuery[ss.selector] = ss.series
		}
	}
	close(errs)

	//Now that we've collected all of the results into `seriesCacheByQuery`
	//we can start processing them.
	newSeries := make([][]prom.Series, len(l.namers))
	for i, namer := range l.namers {
		series, cached := seriesCacheByQuery[namer.Selector()]
		if !cached {
			return result, fmt.Errorf("unable to update list of all metrics: no metrics retrieved for query %q", namer.Selector())
		}
		//Because namers provide a "post-filtering" option, it's not enough to
		//simply take all the series that were produced. We need to further filter them.
		newSeries[i] = namer.SeriesFilterer().FilterSeries(series)
	}

	glog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)

	result.series = newSeries
	result.namers = l.namers
	return result, nil
}

type metricUpdateResult struct {
	series [][]prom.Series
	namers []MetricNamer
}
