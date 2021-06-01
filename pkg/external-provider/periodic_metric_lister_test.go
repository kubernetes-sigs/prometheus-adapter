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
	"testing"
	"time"

	prom "sigs.k8s.io/prometheus-adapter/pkg/client"

	"github.com/stretchr/testify/require"
)

type fakeLister struct {
	callCount int
}

func (f *fakeLister) ListAllMetrics() (MetricUpdateResult, error) {
	f.callCount++

	return MetricUpdateResult{
		series: [][]prom.Series{
			{
				{
					Name: "a_series",
				},
			},
		},
	}, nil
}

func TestWhenNewMetricsAvailableCallbackIsInvoked(t *testing.T) {
	fakeLister := &fakeLister{}
	targetLister, _ := NewPeriodicMetricLister(fakeLister, time.Duration(1000))
	periodicLister := targetLister.(*periodicMetricLister)

	callbackInvoked := false
	callback := func(r MetricUpdateResult) {
		callbackInvoked = true
	}

	periodicLister.AddNotificationReceiver(callback)
	err := periodicLister.updateMetrics()
	require.NoError(t, err)
	require.True(t, callbackInvoked)
}

func TestWhenListingMetricsReturnsCachedValues(t *testing.T) {
	fakeLister := &fakeLister{}
	targetLister, _ := NewPeriodicMetricLister(fakeLister, time.Duration(1000))
	periodicLister := targetLister.(*periodicMetricLister)

	// We haven't invoked the inner lister yet, so we should have no results.
	resultBeforeUpdate, err := periodicLister.ListAllMetrics()
	require.NoError(t, err)
	require.Equal(t, 0, len(resultBeforeUpdate.series))
	require.Equal(t, 0, fakeLister.callCount)

	// We can simulate waiting for the udpate interval to pass...
	// which should result in calling the inner lister to get the metrics.
	err = periodicLister.updateMetrics()
	require.NoError(t, err)
	require.Equal(t, 1, fakeLister.callCount)

	// If we list now, we should return the cached values.
	// Make sure we got some results this time
	// as well as that we didn't unnecessarily invoke the inner lister.
	resultAfterUpdate, err := periodicLister.ListAllMetrics()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(resultAfterUpdate.series))
	require.Equal(t, 1, fakeLister.callCount)
}
