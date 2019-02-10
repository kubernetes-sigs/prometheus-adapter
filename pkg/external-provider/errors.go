package provider

import "errors"

var (
	// ErrorNewOperatorNotSupportedByPrometheus creates an error that represents the fact that we were requested to service a query that
	// Prometheus would be unable to support.
	ErrorNewOperatorNotSupportedByPrometheus = errors.New("operator not supported by prometheus")

	// ErrorNewOperatorRequiresValues creates an error that represents the fact that we were requested to service a query
	// that was malformed in its operator/value combination.
	ErrorNewOperatorRequiresValues = errors.New("operator requires values")

	// ErrorNewOperatorDoesNotSupportValues creates an error that represents the fact that we were requested to service a query
	// that was malformed in its operator/value combination.
	ErrorNewOperatorDoesNotSupportValues = errors.New("operator does not support values")

	// ErrorNewLabelNotSpecified creates an error that represents the fact that we were requested to service a query
	// that was malformed in its label specification.
	ErrorNewLabelNotSpecified = errors.New("label not specified")
)
