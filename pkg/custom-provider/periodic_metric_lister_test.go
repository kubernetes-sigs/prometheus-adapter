package provider

import (
	"testing"
	"time"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/stretchr/testify/require"
)

type fakeLister struct {
	callCount int
}

func (f *fakeLister) ListAllMetrics() (metricUpdateResult, error) {
	f.callCount++

	return metricUpdateResult{
		series: [][]prom.Series{
			[]prom.Series{
				prom.Series{
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
	callback := func(r metricUpdateResult) {
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

	//We haven't invoked the inner lister yet, so we should have no results.
	resultBeforeUpdate, err := periodicLister.ListAllMetrics()
	require.NoError(t, err)
	require.Equal(t, 0, len(resultBeforeUpdate.series))
	require.Equal(t, 0, fakeLister.callCount)

	//We can simulate waiting for the udpate interval to pass...
	//which should result in calling the inner lister to get the metrics.
	err = periodicLister.updateMetrics()
	require.NoError(t, err)
	require.Equal(t, 1, fakeLister.callCount)

	//If we list now, we should return the cached values.
	//Make sure we got some results this time
	//as well as that we didn't unnecessarily invoke the inner lister.
	resultAfterUpdate, err := periodicLister.ListAllMetrics()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(resultAfterUpdate.series))
	require.Equal(t, 1, fakeLister.callCount)
}
