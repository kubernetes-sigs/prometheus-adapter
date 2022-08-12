// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package prometheus provides bindings to the Prometheus HTTP API:
// http://prometheus.io/docs/querying/api/
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"k8s.io/klog/v2"
)

// APIClient is a raw client to the Prometheus Query API.
// It knows how to appropriately deal with generic Prometheus API
// responses, but does not know the specifics of different endpoints.
// You can use this to call query endpoints not represented in Client.
type GenericAPIClient interface {
	// Do makes a request to the Prometheus HTTP API against a particular endpoint.  Query
	// parameters should be in `query`, not `endpoint`.  An error will be returned on HTTP
	// status errors or errors making or unmarshalling the request, as well as when the
	// response has a Status of ResponseError.
	Do(ctx context.Context, verb, endpoint string, query url.Values) (APIResponse, error)
}

// httpAPIClient is a GenericAPIClient implemented in terms of an underlying http.Client.
type httpAPIClient struct {
	client  *http.Client
	baseURL *url.URL
	headers http.Header
}

func (c *httpAPIClient) Do(ctx context.Context, verb, endpoint string, query url.Values) (APIResponse, error) {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)
	var reqBody io.Reader
	if verb == http.MethodGet {
		u.RawQuery = query.Encode()
	} else if verb == http.MethodPost {
		reqBody = strings.NewReader(query.Encode())
	}

	req, err := http.NewRequest(verb, u.String(), reqBody)
	if err != nil {
		return APIResponse{}, fmt.Errorf("error constructing HTTP request to Prometheus: %v", err)
	}
	req.WithContext(ctx)
	for key, values := range c.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if verb == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.client.Do(req)
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if err != nil {
		return APIResponse{}, err
	}

	if klog.V(6).Enabled() {
		klog.Infof("%s %s %s", verb, u.String(), resp.Status)
	}

	code := resp.StatusCode

	// codes that aren't 2xx, 400, 422, or 503 won't return JSON objects
	if code/100 != 2 && code != 400 && code != 422 && code != 503 {
		return APIResponse{}, &Error{
			Type: ErrBadResponse,
			Msg:  fmt.Sprintf("unknown response code %d", code),
		}
	}

	var body io.Reader = resp.Body
	if klog.V(8).Enabled() {
		data, err := ioutil.ReadAll(body)
		if err != nil {
			return APIResponse{}, fmt.Errorf("unable to log response body: %v", err)
		}
		klog.Infof("Response Body: %s", string(data))
		body = bytes.NewReader(data)
	}

	var res APIResponse
	if err = json.NewDecoder(body).Decode(&res); err != nil {
		return APIResponse{}, &Error{
			Type: ErrBadResponse,
			Msg:  err.Error(),
		}
	}

	if res.Status == ResponseError {
		return res, &Error{
			Type: res.ErrorType,
			Msg:  res.Error,
		}
	}

	return res, nil
}

// NewGenericAPIClient builds a new generic Prometheus API client for the given base URL and HTTP Client.
func NewGenericAPIClient(client *http.Client, baseURL *url.URL, headers http.Header) GenericAPIClient {
	return &httpAPIClient{
		client:  client,
		baseURL: baseURL,
		headers: headers,
	}
}

const (
	queryURL      = "/api/v1/query"
	queryRangeURL = "/api/v1/query_range"
	seriesURL     = "/api/v1/series"
)

// queryClient is a Client that connects to the Prometheus HTTP API.
type queryClient struct {
	api  GenericAPIClient
	verb string
}

// NewClientForAPI creates a Client for the given generic Prometheus API client.
func NewClientForAPI(client GenericAPIClient, verb string) Client {
	return &queryClient{
		api:  client,
		verb: verb,
	}
}

// NewClient creates a Client for the given HTTP client and base URL (the location of the Prometheus server).
func NewClient(client *http.Client, baseURL *url.URL, headers http.Header, verb string) Client {
	genericClient := NewGenericAPIClient(client, baseURL, headers)
	return NewClientForAPI(genericClient, verb)
}

func (h *queryClient) Series(ctx context.Context, interval model.Interval, selectors ...Selector) ([]Series, error) {
	vals := url.Values{}
	if interval.Start != 0 {
		vals.Set("start", interval.Start.String())
	}
	if interval.End != 0 {
		vals.Set("end", interval.End.String())
	}

	for _, selector := range selectors {
		vals.Add("match[]", string(selector))
	}

	res, err := h.api.Do(ctx, h.verb, seriesURL, vals)
	if err != nil {
		return nil, err
	}

	var seriesRes []Series
	err = json.Unmarshal(res.Data, &seriesRes)
	return seriesRes, err
}

func (h *queryClient) Query(ctx context.Context, t model.Time, query Selector) (QueryResult, error) {
	vals := url.Values{}
	vals.Set("query", string(query))
	if t != 0 {
		vals.Set("time", t.String())
	}
	if timeout, hasTimeout := timeoutFromContext(ctx); hasTimeout {
		vals.Set("timeout", model.Duration(timeout).String())
	}

	res, err := h.api.Do(ctx, h.verb, queryURL, vals)
	if err != nil {
		return QueryResult{}, err
	}

	var queryRes QueryResult
	err = json.Unmarshal(res.Data, &queryRes)
	return queryRes, err
}

func (h *queryClient) QueryRange(ctx context.Context, r Range, query Selector) (QueryResult, error) {
	vals := url.Values{}
	vals.Set("query", string(query))

	if r.Start != 0 {
		vals.Set("start", r.Start.String())
	}
	if r.End != 0 {
		vals.Set("end", r.End.String())
	}
	if r.Step != 0 {
		vals.Set("step", model.Duration(r.Step).String())
	}
	if timeout, hasTimeout := timeoutFromContext(ctx); hasTimeout {
		vals.Set("timeout", model.Duration(timeout).String())
	}

	res, err := h.api.Do(ctx, h.verb, queryRangeURL, vals)
	if err != nil {
		return QueryResult{}, err
	}

	var queryRes QueryResult
	err = json.Unmarshal(res.Data, &queryRes)
	return queryRes, err
}

// timeoutFromContext checks the context for a deadline and calculates a "timeout" duration from it,
// when present
func timeoutFromContext(ctx context.Context) (time.Duration, bool) {
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		return time.Now().Sub(deadline), true
	}

	return time.Duration(0), false
}
