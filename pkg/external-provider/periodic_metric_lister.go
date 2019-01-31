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
	mostRecentResult MetricUpdateResult
	callbacks        []MetricUpdateCallback
}

// NewPeriodicMetricLister creates a MetricLister that periodically pulls the list of available metrics
// at the provided interval, but defers the actual act of retrieving the metrics to the supplied MetricLister.
func NewPeriodicMetricLister(realLister MetricLister, updateInterval time.Duration) (MetricListerWithNotification, Runnable) {
	lister := periodicMetricLister{
		updateInterval: updateInterval,
		realLister:     realLister,
		callbacks:      make([]MetricUpdateCallback, 0),
	}

	return &lister, &lister
}

func (l *periodicMetricLister) AddNotificationReceiver(callback MetricUpdateCallback) {
	l.callbacks = append(l.callbacks, callback)
}

func (l *periodicMetricLister) ListAllMetrics() (MetricUpdateResult, error) {
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
	//Let our listeners know we've got new data ready for them.
	l.notifyListeners()
	return nil
}

func (l *periodicMetricLister) notifyListeners() {
	for _, listener := range l.callbacks {
		if listener != nil {
			listener(l.mostRecentResult)
		}
	}
}

func (l *periodicMetricLister) UpdateNow() {
	l.updateMetrics()
}
