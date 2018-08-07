Kubernetes Custom Metrics Adapter for Prometheus
================================================

[![Build Status](https://travis-ci.org/DirectXMan12/k8s-prometheus-adapter.svg?branch=master)](https://travis-ci.org/DirectXMan12/k8s-prometheus-adapter)

This repository contains an implementation of the Kubernetes custom
metrics API
([custom.metrics.k8s.io/v1beta1](https://github.com/kubernetes/metrics/tree/master/pkg/apis/custom_metrics)),
suitable for use with the autoscaling/v2 Horizontal Pod Autoscaler in
Kubernetes 1.6+.

Quick Links
-----------

- [Config walkthrough](docs/config-walkthrough.md) and [config reference](docs/config.md).
- [End-to-end walkthrough](docs/walkthrough.md)
- [Deployment info and files](deploy/README.md)

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
  update the cache of available metrics from Prometheus.  Since the adapter
  only lists metrics during discovery that exist between the current time and
  the last discovery query, your relist interval should be equal to or larger
  than your Prometheus scrape interval, otherwise your metrics will
  occaisonally disappear from the adapter.

- `--prometheus-url=<url>`: This is the URL used to connect to Prometheus.
  It will eventually contain query parameters to configure the connection.

- `--config=<yaml-file>` (`-c`): This configures how the adapter discovers available
  Prometheus metrics and the associated Kubernetes resources, and how it presents those
  metrics in the custom metrics API.  More information about this file can be found in
  [docs/config.md](docs/config.md).

Presentation
------------

The adapter gathers the names of available metrics from Prometheus
a regular interval (see [Configuration](#configuration) above), and then
only exposes metrics that follow specific forms.

The rules governing this discovery are specified in a [configuration file](docs/config.md).
If you were relying on the implicit rules from the previous version of the adapter,
you can use the included `config-gen` tool to generate a configuration that matches
the old implicit ruleset:

```shell
$ go run cmd/config-gen main.go [--rate-interval=<duration>] [--label-prefix=<prefix>]
```

Example
-------

A brief walkthrough exists in [docs/walkthrough.md](docs/walkthrough.md).

Additionally, [@luxas](https://github.com/luxas) has an excellent example
deployment of Prometheus, this adapter, and a demo pod which serves
a metric `http_requests_total`, which becomes the custom metrics API
metric `pods/http_requests`.  It also autoscales on that metric using the
`autoscaling/v2beta1` HorizontalPodAutoscaler.  Note that @luxas's tutorial
uses a slightly older version of the adapter.

It can be found at https://github.com/luxas/kubeadm-workshop.  Pay special
attention to:

- [Deploying the Prometheus
  Operator](https://github.com/luxas/kubeadm-workshop#deploying-the-prometheus-operator-for-monitoring-services-in-the-cluster)
- [Setting up the custom metrics adapter and sample
  app](https://github.com/luxas/kubeadm-workshop#deploying-a-custom-metrics-api-server-and-a-sample-app)
