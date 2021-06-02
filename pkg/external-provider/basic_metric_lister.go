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

	"k8s.io/klog/v2"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"

	pmodel "github.com/prometheus/common/model"
)

// Runnable represents something that can be run until told to stop.
type Runnable interface {
	// Run runs the runnable forever.
	Run()
	// RunUntil runs the runnable until the given channel is closed.
	RunUntil(stopChan <-chan struct{})
}

// A MetricLister provides a window into all of the metrics that are available within a given
// Prometheus instance, classified as either Custom or External metrics, but presented generically
// so that it can manage both types simultaneously.
type MetricLister interface {
	ListAllMetrics() (MetricUpdateResult, error)
}

// A MetricListerWithNotification is a MetricLister that has the ability to notify listeners
// when new metric data is available.
type MetricListerWithNotification interface {
	MetricLister
	Runnable

	// AddNotificationReceiver registers a callback to be invoked when new metric data is available.
	AddNotificationReceiver(MetricUpdateCallback)
	// UpdateNow forces an immediate refresh from the source data. Primarily for test purposes.
	UpdateNow()
}

type basicMetricLister struct {
	promClient prom.Client
	namers     []naming.MetricNamer
	lookback   time.Duration
}

// NewBasicMetricLister creates a MetricLister that is capable of interactly directly with Prometheus to list metrics.
func NewBasicMetricLister(promClient prom.Client, namers []naming.MetricNamer, lookback time.Duration) MetricLister {
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

func (l *basicMetricLister) ListAllMetrics() (MetricUpdateResult, error) {
	result := MetricUpdateResult{
		series: make([][]prom.Series, 0),
		namers: make([]naming.MetricNamer, 0),
	}

	startTime := pmodel.Now().Add(-1 * l.lookback)

	// these can take a while on large clusters, so launch in parallel
	// and don't duplicate
	selectors := make(map[prom.Selector]struct{})
	selectorSeriesChan := make(chan selectorSeries, len(l.namers))
	errs := make(chan error, len(l.namers))
	for _, converter := range l.namers {
		sel := converter.Selector()
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
			// Push into the channel: "this selector produced these series"
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
	// for each converter. So here, we'll assume that we should receive one item per converter.
	for range l.namers {
		if err := <-errs; err != nil {
			return result, fmt.Errorf("unable to update list of all metrics: %v", err)
		}
		// Receive from the channel: "this selector produced these series"
		// We stuff that into this map so that we can collect the data as it arrives
		// and then, once we've received it all, we can process it below.
		if ss := <-selectorSeriesChan; ss.series != nil {
			seriesCacheByQuery[ss.selector] = ss.series
		}
	}
	close(errs)

	// Now that we've collected all of the results into `seriesCacheByQuery`
	// we can start processing them.
	newSeries := make([][]prom.Series, len(l.namers))
	for i, namer := range l.namers {
		series, cached := seriesCacheByQuery[namer.Selector()]
		if !cached {
			return result, fmt.Errorf("unable to update list of all metrics: no metrics retrieved for query %q", namer.Selector())
		}
		// Because converters provide a "post-filtering" option, it's not enough to
		// simply take all the series that were produced. We need to further filter them.
		newSeries[i] = namer.FilterSeries(series)
	}

	klog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)

	result.series = newSeries
	result.namers = l.namers
	return result, nil
}

// MetricUpdateResult represents the output of a periodic inspection of metrics found to be
// available in Prometheus.
// It includes both the series data the Prometheus exposed, as well as the configurational
// object that led to their discovery.
type MetricUpdateResult struct {
	series [][]prom.Series
	namers []naming.MetricNamer
}

// MetricUpdateCallback is a function signature for receiving periodic updates about
// available metrics.
type MetricUpdateCallback func(MetricUpdateResult)
