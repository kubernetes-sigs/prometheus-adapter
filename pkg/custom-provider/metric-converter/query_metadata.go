package provider

//QueryMetadata is a data object the holds information about what inputs
//were used to generate Prometheus query results. In most cases it's not
//necessary, as the Prometheus result come back with enough information
//to determine the metric name. However, for scalar results, Prometheus
//only provides the value.
type QueryMetadata struct {
	MetricName      string
	WindowInSeconds int64
	//TODO: Type this?
	Aggregation string
}
