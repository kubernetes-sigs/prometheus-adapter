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

package naming

import "errors"

var (
	// ErrUnsupportedOperator creates an error that represents the fact that we were requested to service a query that
	// Prometheus would be unable to support.
	ErrUnsupportedOperator = errors.New("operator not supported by prometheus")

	// ErrMalformedQuery creates an error that represents the fact that we were requested to service a query
	// that was malformed in its operator/value combination.
	ErrMalformedQuery = errors.New("operator requires values")

	// ErrQueryUnsupportedValues creates an error that represents an unsupported return value from the
	// specified query.
	ErrQueryUnsupportedValues = errors.New("operator does not support values")

	// ErrLabelNotSpecified creates an error that represents the fact that we were requested to service a query
	// that was malformed in its label specification.
	ErrLabelNotSpecified = errors.New("label not specified")
)
