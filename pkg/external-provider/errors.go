package provider

import "errors"

// NewOperatorNotSupportedByPrometheusError creates an error that represents the fact that we were requested to service a query that
// Prometheus would be unable to support.
func NewOperatorNotSupportedByPrometheusError() error {
	return errors.New("operator not supported by prometheus")
}

// NewOperatorRequiresValuesError creates an error that represents the fact that we were requested to service a query
// that was malformed in its operator/value combination.
func NewOperatorRequiresValuesError() error {
	return errors.New("operator requires values")
}

// NewOperatorDoesNotSupportValuesError creates an error that represents the fact that we were requested to service a query
// that was malformed in its operator/value combination.
func NewOperatorDoesNotSupportValuesError() error {
	return errors.New("operator does not support values")
}

// NewLabelNotSpecifiedError creates an error that represents the fact that we were requested to service a query
// that was malformed in its label specification.
func NewLabelNotSpecifiedError() error {
	return errors.New("label not specified")
}
