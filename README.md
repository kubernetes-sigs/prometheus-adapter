Kubernetes Custom Metrics Adapter for Prometheus
================================================

This repository contains an implementation of the Kubernetes custom
metrics API
([custom-metrics.metrics.k8s.io/v1alpha1](https://github.com/kubernetes/metrics/tree/master/pkg/apis/custom_metrics)),
suitable for use with the autoscaling/v2 Horizontal Pod Autoscaler in
Kubernetes 1.6+.

Configuration
-------------

The adapter takes the standard Kubernetes generic API server arguments
(including those for authentication and authorization).  By default, it
will attempt to using [Kubernetes in-cluster
config](https://kubernetes.io/docs/tasks/access-application-cluster/access-cluster/#accessing-the-api-from-a-pod)
to connect to the cluster.

It takes the following addition arguments specific to configuring how the
adapter talks to Prometheus and the main Kubernetes cluster:

- `--lister-kubeconfig=<path-to-kubeconfig>`: This configures
  how the adapter talks to a Kubernetes API server in order to list
  objects when operating with label selectors.  By default, it will use
  in-cluster config.

- `--metrics-relist-interval=<duration>`: This is the interval at which to
  update the cache of available metrics from Prometheus.

- `--rate-interval=<duration>`: This is the duration used when requesting
  rate metrics from Prometheus.  It *must* be larger than your Prometheus
  collection interval.

- `--prometheus-url=<url>`: This is the URL used to connect to Prometheus.
  It will eventually contain query parameters to configure the connection.

Presentation
------------

The adapter gathers the names of available metrics from Prometheus at the
specified interval.  Only metrics of the following forms are considered:

- "container" metrics (cAdvisor container metrics): series with a name
  starting with `container_`, as well as non-empty `namespace` and
  `pod_name` labels.

- "namespaced" metrics (metrics describing namespaced Kubernetes objects):
  series with non-empty namespace labels (which don't start with
  `container_`).

*Note*: Currently, metrics on non-namespaced objects (besides namespaces
themselves) are not supported.

Metrics in Prometheus are converted in the custom-metrics-API metrics as
follows:

1. The metric name and type are decided:
   - For container metrics, the `container_` prefix is removed
   - If the metric has the `_total` suffix, it is marked as a counter
     metric, and the suffix is removed
   - If the metric has the `_seconds_total` suffix, it is marked as
     a seconds counter metric, and the suffix is removed.
   - If the metric has none of the above suffixes, is is marked as a gauge
    metric, and the metric name is used as-is

2. Relevant resources are associated with the metric:
   - container metrics are associated with pods only
   - for non-container metrics, each label on the series is considered. If
     that label represents a resource (without the group) available on the
     server, the metric is associated with that resource.  A metric may be
     associated with multiple resources.

When retrieving counter and seconds-counter metrics, the adapter requests
the metrics as a rate over the configured amount of time.  For metrics
with multiple associated resources, the adapter requests the metric
aggregated over all non-requested metrics.

The adapter does not consider resources consumed by the "POD" container,
which exists as part of all Kubernetes pods running in Docker simply
supports the existance of the pod's shared network namespace.
