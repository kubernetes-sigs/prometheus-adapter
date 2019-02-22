package provider

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
