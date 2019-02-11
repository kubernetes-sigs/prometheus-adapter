package provider

import "errors"

var (
	// ErrNewOperatorNotSupportedByPrometheus creates an error that represents the fact that we were requested to service a query that
	// Prometheus would be unable to support.
	ErrNewOperatorNotSupportedByPrometheus = errors.New("operator not supported by prometheus")

	// ErrNewOperatorRequiresValues creates an error that represents the fact that we were requested to service a query
	// that was malformed in its operator/value combination.
	ErrNewOperatorRequiresValues = errors.New("operator requires values")

	// ErrNewOperatorDoesNotSupportValues creates an error that represents the fact that we were requested to service a query
	// that was malformed in its operator/value combination.
	ErrNewOperatorDoesNotSupportValues = errors.New("operator does not support values")

	// ErrNewLabelNotSpecified creates an error that represents the fact that we were requested to service a query
	// that was malformed in its label specification.
	ErrNewLabelNotSpecified = errors.New("label not specified")
)
