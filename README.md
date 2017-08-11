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

The adapter gathers the names of available metrics from Prometheus
a regular interval (see [Configuration](#configuration) above), and then
only exposes metrics that follow specific forms.

In general:

- Metrics must have the `namespace` label to be considered.

- For each label on a metric, if that label name corresponds to
  a Kubernetes resource (like `pod` or `service`), the metric will be
  associated with that resource.

- Metrics ending in `_total` are assumed to be cumulative, and will be
  exposed without the suffix as a rate metric.

Detailed information can be found under [docs/format.md](docs/format.md).

Example
-------

A brief walkthrough exists in [docs/walkthrough.md](docs/walkthrough.md).

Additionally, [@luxas](https://github.com/luxas) has an excellent example
deployment of Prometheus, this adapter, and a demo pod which serves
a metric `http_requests_total`, which becomes the custom metrics API
metric `pods/http_requests`.  It also autoscales on that metric using the
`autoscaling/v2alpha1` HorizontalPodAutoscaler.

It can be found at https://github.com/luxas/kubeadm-workshop.  Pay special
attention to:

- [Deploying the Prometheus
  Operator](https://github.com/luxas/kubeadm-workshop#deploying-the-prometheus-operator-for-monitoring-services-in-the-cluster)
- [Setting up the custom metrics adapter and sample
  app](https://github.com/luxas/kubeadm-workshop#deploying-a-custom-metrics-api-server-and-a-sample-app)
