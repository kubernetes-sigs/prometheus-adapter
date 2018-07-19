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
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

type periodicMetricLister struct {
	realLister       MetricLister
	updateInterval   time.Duration
	mostRecentResult metricUpdateResult
	callback         func(metricUpdateResult)
}

//NewPeriodicMetricLister creates a MetricLister that periodically pulls the list of available metrics
//at the provided interval, but defers the actual act of retrieving the metrics to the supplied MetricLister.
func NewPeriodicMetricLister(realLister MetricLister, updateInterval time.Duration) (MetricListerWithNotification, Runnable) {
	lister := periodicMetricLister{
		updateInterval: updateInterval,
		realLister:     realLister,
	}

	return &lister, &lister
}

func (l *periodicMetricLister) SetNotificationReceiver(callback func(metricUpdateResult)) {
	l.callback = callback
}

func (l *periodicMetricLister) ListAllMetrics() (metricUpdateResult, error) {
	return l.mostRecentResult, nil
}

func (l *periodicMetricLister) Run() {
	l.RunUntil(wait.NeverStop)
}

func (l *periodicMetricLister) RunUntil(stopChan <-chan struct{}) {
	go wait.Until(func() {
		if err := l.updateMetrics(); err != nil {
			utilruntime.HandleError(err)
		}
	}, l.updateInterval, stopChan)
}

func (l *periodicMetricLister) updateMetrics() error {
	result, err := l.realLister.ListAllMetrics()

	if err != nil {
		return err
	}

	//Cache the result.
	l.mostRecentResult = result
	//Let our listener know we've got new data ready for them.
	if l.callback != nil {
		l.callback(result)
	}
	return nil
}

// func (l *periodicMetricLister) updateMetrics() (metricUpdateResult, error) {

// 	result := metricUpdateResult{
// 		series: make([][]prom.Series, 0),
// 		namers: make([]MetricNamer, 0),
// 	}

// 	startTime := pmodel.Now().Add(-1 * l.updateInterval)

// 	// these can take a while on large clusters, so launch in parallel
// 	// and don't duplicate
// 	selectors := make(map[prom.Selector]struct{})
// 	selectorSeriesChan := make(chan selectorSeries, len(l.namers))
// 	errs := make(chan error, len(l.namers))
// 	for _, namer := range l.namers {
// 		sel := namer.Selector()
// 		if _, ok := selectors[sel]; ok {
// 			errs <- nil
// 			selectorSeriesChan <- selectorSeries{}
// 			continue
// 		}
// 		selectors[sel] = struct{}{}
// 		go func() {
// 			series, err := l.promClient.Series(context.TODO(), pmodel.Interval{startTime, 0}, sel)
// 			if err != nil {
// 				errs <- fmt.Errorf("unable to fetch metrics for query %q: %v", sel, err)
// 				return
// 			}
// 			errs <- nil
// 			selectorSeriesChan <- selectorSeries{
// 				selector: sel,
// 				series:   series,
// 			}
// 		}()
// 	}

// 	// don't do duplicate queries when it's just the matchers that change
// 	seriesCacheByQuery := make(map[prom.Selector][]prom.Series)

// 	// iterate through, blocking until we've got all results
// 	for range l.namers {
// 		if err := <-errs; err != nil {
// 			return result, fmt.Errorf("unable to update list of all metrics: %v", err)
// 		}
// 		if ss := <-selectorSeriesChan; ss.series != nil {
// 			seriesCacheByQuery[ss.selector] = ss.series
// 		}
// 	}
// 	close(errs)

// 	newSeries := make([][]prom.Series, len(l.namers))
// 	for i, namer := range l.namers {
// 		series, cached := seriesCacheByQuery[namer.Selector()]
// 		if !cached {
// 			return result, fmt.Errorf("unable to update list of all metrics: no metrics retrieved for query %q", namer.Selector())
// 		}
// 		newSeries[i] = namer.FilterSeries(series)
// 	}

// 	glog.V(10).Infof("Set available metric list from Prometheus to: %v", newSeries)

// 	result.series = newSeries
// 	result.namers = l.namers
// 	return result, nil
// }
