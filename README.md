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

FAQs
----

### Why do my metrics keep jumping between a normal value and a very large number?

You're probably switching between whole numbers (e.g. `10`) and milli-quantities (e.g. `10500m`).
Worry not!  This is just how Kubernetes represents fractional values.  See the
[Quantity Values](/docs/walkthrough.md#quantity-values) section of the walkthrough for a bit more
information.

### Why isn't my metric showing up?

First, check your configuration.  Does it select your metric?  You can
find the [default configuration](/deploy/custom-metrics-config-map.yaml)
in the deploy directory, and more information about configuring the
adapter in the [docs](/docs/config.md).

Next, check if the discovery information looks right.  You should see the
metrics showing up as associated with the resources you expect at
`/apis/custom.metrics.k8s.io/v1beta1/` (you can use `kubectl get --raw
/apis/custom.metrics.k8s.io/v1beta1` to check, and can pipe to `jq` to
pretty-print the results, if you have it installed). If not, make sure
your series are labeled correctly.  Consumers of the custom metrics API
(especially the HPA) don't do any special logic to associate a particular
resource to a particular series, so you have to make sure that the adapter
does it instead.

For example, if you want a series `foo` to be associated with deployment
`bar` in namespace `somens`, make sure there's some label that represents
deployment name, and that the adapter is configured to use it.  With the
default config, that means you'd need the query
`foo{namespace="somens",deployment="bar"}` to return some results in
Prometheus.

Next, try using the `--v=6` flag on the adapter to see the exact queries
being made by the adapter.  Try url-decoding the query and pasting it into
the Prometheus web console to see if the query looks wrong.

### My query contains multiple metrics, how do I make that work?

It's actually fairly straightforward, if a bit non-obvious.  Simply choose one
metric to act as the "discovery" and "naming" metric, and use that to configure
the "discovery" and "naming" parts of the configuration.  Then, you can write
whichever metrics you want in the `metricsQuery`!  The series query can contain
whichever metrics you want, as long as they have the right set of labels.

For example, if you have two metrics `foo_total` and `foo_count`, you might write

```yaml
rules:
- seriesQuery: 'foo_total'
  resources: {overrides: {system_name: {resource: "node"}}}
  name:
    matches: 'foo_total'
    as: 'foo'
  metricsQuery: 'sum(foo_total) by (<<.GroupBy>>) / sum(foo_count) by (<<.GroupBy>>)'
```

### I get errors about SubjectAccessReviews/system:anonymous/TLS/Certificates/RequestHeader!

It's important to understand the role of TLS in the Kubernetes cluster.  There's a high-level
overview here: https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/auth.md.

All of the above errors generally boil down to misconfigured certificates.
Specifically, you'll need to make sure your cluster's aggregation layer is
properly configured, with requestheader certificates set up properly.

Errors about SubjectAccessReviews failing for system:anonymous generally mean
that your cluster's given requestheader CA doesn't trust the proxy certificates
from the API server aggregator.

On the other hand, if you get an error from the aggregator about invalid certificates,
it's probably because the CA specified in the `caBundle` field of your APIService
object doesn't trust the serving certificates for the adapter.

If you're seeing SubjectAccessReviews failures for non-anonymous users, check your
RBAC rules -- you probably haven't given users permission to operate on resources in
the `custom.metrics.k8s.io` API group.

### My metrics appear and disappear

You probably have a Prometheus collection interval or computation interval
that's larger than your adapter's discovery interval.  If the metrics
appear in discovery but occaisionally return not-found, those intervals
are probably larger than one of the rate windows used in one of your
queries.  The adapter only considers metrics with datapoints in the window
`[now-discoveryInterval, now]` (in order to only capture metrics that are
still present), so make sure that your discovery interval is at least as
large as your collection interval.
