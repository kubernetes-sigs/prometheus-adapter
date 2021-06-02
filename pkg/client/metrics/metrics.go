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
package metrics

import (
	"context"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/prometheus-adapter/pkg/client"
)

var (
	// queryLatency is the total latency of any query going through the
	// various endpoints (query, range-query, series).  It includes some deserialization
	// overhead and HTTP overhead.
	queryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cmgateway_prometheus_query_latency_seconds",
			Help:    "Prometheus client query latency in seconds.  Broken down by target prometheus endpoint and target server",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10),
		},
		[]string{"endpoint", "server"},
	)
)

func init() {
	prometheus.MustRegister(queryLatency)
}

// instrumentedClient is a client.GenericAPIClient which instruments calls to Do,
// capturing request latency.
type instrumentedGenericClient struct {
	serverName string
	client     client.GenericAPIClient
}

func (c *instrumentedGenericClient) Do(ctx context.Context, verb, endpoint string, query url.Values) (client.APIResponse, error) {
	startTime := time.Now()
	var err error
	defer func() {
		endTime := time.Now()
		// skip calls where we don't make the actual request
		if err != nil {
			if _, wasAPIErr := err.(*client.Error); !wasAPIErr {
				// TODO: measure API errors by code?
				return
			}
		}
		queryLatency.With(prometheus.Labels{"endpoint": endpoint, "server": c.serverName}).Observe(endTime.Sub(startTime).Seconds())
	}()

	var resp client.APIResponse
	resp, err = c.client.Do(ctx, verb, endpoint, query)
	return resp, err
}

func InstrumentGenericAPIClient(client client.GenericAPIClient, serverName string) client.GenericAPIClient {
	return &instrumentedGenericClient{
		serverName: serverName,
		client:     client,
	}
}
