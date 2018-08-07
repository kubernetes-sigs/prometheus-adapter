Walkthrough
===========

This walkthrough will go over the basics of setting up the Prometheus
adapter on your cluster and configuring an autoscaler to use application
metrics sourced from the adapter.

Prerequisites
-------------

### Cluster Configuration ###

Before getting started, ensure that the main components of your
cluster are configured for autoscaling on custom metrics.  As of
Kubernetes 1.7, this requires enabling the aggregation layer on the API
server and configuring the controller manager to use the metrics APIs via
their REST clients.

Detailed instructions can be found in the Kubernetes documentation under
[Horizontal Pod
Autoscaling](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-custom-metrics).

Make sure that you've properly configured metrics-server (as is default in
Kubernetes 1.9+), or enabling custom metrics autoscaling support will
disable CPU autoscaling support.

Note that most of the API versions in this walkthrough target Kubernetes
1.9+.  Note that current versions of the adapter *only* work with
Kubernetes 1.8+.  Version 0.1.0 works with Kubernetes 1.7, but is
significantly different.

### Binaries and Images ###

In order to follow this walkthrough, you'll need container images for
Prometheus and the custom metrics adapter.

Prometheus can be found at `prom/prometheus` on Dockerhub.  The adapter
has different images for each arch, and can be found at
`directxman12/k8s-prometheus-adapter-${ARCH}`.  For instance, if you're on
an x86_64 machine, use the `directxman12/k8s-prometheus-adapter-amd64`
image.

If you're feeling adventurous, you can build the latest version of the
custom metrics adapter by running `make docker-build`.

Launching Prometheus and the Adapter
------------------------------------

In this walkthrough, it's assumed that you're deploying Prometheus into
its own namespace called `prom`.  Most of the sample commands and files
are namespace-agnostic, but there are a few commands that rely on
namespace.  If you're using a different namespace, simply substitute that
in for `prom` when it appears.

### Prometheus Configuration ###

It's reccomended to use the [Prometheus
Operator](https://coreos.com/operators/prometheus/docs/latest/) to deploy
Prometheus.  It's a lot easier than configuring Prometheus by hand.  Note
that the Prometheus operator rules rename some labels if they conflict
with its automatic labels, so you may have to tweak the adapter
configuration slightly.

If you don't want to use the Prometheus Operator, you'll have to deploy
Prometheus with a hand-written configuration.  Below, you can find the
relevant parts of the configuration that are expected for this
walkthrough.  See the Prometheus documentation on [configuring
Prometheus](https://prometheus.io/docs/operating/configuration/) for more
information.

For the purposes of this walkthrough, you'll need the following
configuration options to be set:

<details>

<summary>prom-cfg.yaml</summary>

```yaml
# a short scrape interval means you can respond to changes in
# metrics more quickly
global:
  scrape_interval: 15s

# you need a scrape configuration for scraping from pods
scrape_configs:
- job_name: 'kubernetes-pods'
  # if you want to use metrics on jobs, set the below field to
  # true to prevent Prometheus from setting the `job` label
  # automatically.
  honor_labels: false
  kubernetes_sd_configs:
  - role: pod
  # skip verification so you can do HTTPS to pods
  tls_config:
    insecure_skip_verify: true
  # make sure your labels are in order
  relabel_configs:
  # these labels tell Prometheus to automatically attach source
  # pod and namespace information to each collected sample, so
  # that they'll be exposed in the custom metrics API automatically.
  - source_labels: [__meta_kubernetes_namespace]
    action: replace
    target_label: namespace
  - source_labels: [__meta_kubernetes_pod_name]
    action: replace
    target_label: pod
  # these labels tell Prometheus to look for
  # prometheus.io/{scrape,path,port} annotations to configure
  # how to scrape
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
    action: keep
    regex: true
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
    action: replace
    target_label: __metrics_path__
    regex: (.+)
  - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
    action: replace
    regex: ([^:]+)(?::\d+)?;(\d+)
    replacement: $1:$2
    target_label: __address__
  - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scheme]
    action: replace
    target_label: __scheme__
    regex: (.+)
```

</details>

Ensure that your Prometheus is up and running by accessing the Prometheus
dashboard, and checking on the labels on those metrics.  You'll need the
label names for configuring the adapter.

### Creating the Resources and Launching the Deployment ###

The [deploy/manifests](/deploy/manifests) directory contains the
appropriate files for creating the Kubernetes objects to deploy the
adapter.

See the [deployment README](/deploy/README.md) for more information about
the steps to deploy the adapter.  Note that if you're deploying on
a non-x86_64 (amd64) platform, you'll need to change the `image` field in
the Deployment to be the appropriate image for your platform.

You may also need to modify the ConfigMap containing the metrics discovery
configuration.  If you're using the Prometheus configuration described
above, it should work out of the box in common cases.  Otherwise, read the
[configuration documentation](/docs/config.md) to learn how to configure
the adapter for your particular metrics and labels.  The [configuration
walkthrough](/docs/config-walkthrough.md) gives an end-to-end
configuration tutorial for configure the adapter for a scenario similar to
this one.

### The Registered API ###

As part of the creation of the adapter Deployment and associated objects
(performed above), we registered the API with the API aggregator (part of
the main Kubernetes API server).

The API is registered as `custom.metrics.k8s.io/v1beta1`, and you can find
more information about aggregation at [Concepts:
Aggregation](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/aggregation.md).

If you're deploying into production, you'll probably want to modify the
APIService object to contain the CA used to sign your serving
certificates.

To do this, first base64-encode the CA (assuming it's stored in
/tmp/ca.crt):

```shell
$ base64 -w 0 < /tmp/ca.crt
```

Then, edit the APIService and place the encoded contents into the
`caBundle` field under `spec`, and removing the `insecureSkipTLSVerify`
field in the same location:

```shell
$ kubectl edit apiservice v1beta1.custom.metrics.k8s.io
```

This ensures that the API aggregator checks that the API is being served
by the server that you expect, by verifying the certificates.

### Double-Checking Your Work ###

With that all set, your custom metrics API should show up in discovery.

Try fetching the discovery information for it:

```shell
$ kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1
```

Since you don't have any metrics collected yet, you shouldn't see any
available resources, but the request should return successfully.  Keep
this command in mind -- you'll want to use it later once you have a pod
producing custom metrics.

Collecting Application Metrics
------------------------------

Now that you have a working pipeline for ingesting application metrics,
you'll need an application that produces some metrics.  Any application
which produces Prometheus-formatted metrics will do.  For the purposes of
this walkthrough, try out [@luxas](https://github.com/luxas)'s simple HTTP
counter in the `luxas/autoscale-demo` image on Dockerhub:

<details>

<summary>sample-app.deploy.yaml</summary>

```yaml
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: sample-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sample-app
  template:
    metadata:
      labels:
        app: sample-app
      annotations:
        # if you're not using the Operator, you'll need these annotations
        # otherwise, configure the operator to collect metrics from
        # the sample-app service on port 80 at /metrics
        prometheus.io/scrape: true
        prometheus.io/port: 8080
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - image: luxas/autoscale-demo:v0.1.2
        name: metrics-provider
      ports:
      - name: http
        port: 8080
```

</details>

Create this deployment, and expose it so that you can easily trigger
increases in metrics:

```yaml
$ kubectl create -f sample-app.deploy.yaml
$ kubectl create service clusterip sample-app --tcp=80:8080
```

This sample application provides some metrics on the number of HTTP
requests it receives.  Consider the metric `http_requests_total`.  First,
check that it appears in discovery using the command from [Double-Checking
Yor Work](#double-checking-your-work).  The cumulative Prometheus metric
`http_requests_total` should have become the custom-metrics-API rate
metric `pods/http_requests`.  Check out its value:

```shell
$ kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta1/namespaces/default/pods/*/http_requests?selector=app%3Dsample-app"
```

It should be zero, since you're not currently accessing it.  Now, create
a few requests with curl:

```shell
$ curl http://$(kubectl get service sample-app -o jsonpath='{ .spec.clusterIP }')/metrics
```

Try fetching the metrics again.  You should see an increase in the rate
after the collection interval specified in your Prometheus configuration
has elapsed.  If you leave it for a bit, the rate will go back down again.

Notice that the API uses Kubernetes-style quantities to describe metric
values.  These quantities use SI suffixes instead of decimal points.  The
most common to see in the metrics API is the `m` suffix, which means
milli-units, or 1000ths of a unit.  If your metric is exactly a whole
number of units on the nose, you might not see a suffix. Otherwise, you'll
probably see an `m` suffix to represent fractions of a unit.

For example, here, `500m` would be half a request per second, `10` would
be 10 requests per second, and `10500m` would be `10.5` requests per
second.


### Troubleshooting Missing Metrics

If the metric does not appear, or is not registered with the right
resources, you might need to modify your [metrics discovery
configuration](/docs/config.md), as mentioned above.  Check your labels via
the Prometheus dashboard, and then modify the configuration appropriately.

As noted in the main [README](/README.md), you'll need to also make sure
your metrics relist interval is at least your Prometheus scrape interval.
If it's less that that, you'll see metrics periodically appear and
disappear from the adapter.

Autoscaling
-----------

Now that you have an application which produces custom metrics, you'll be
able to autoscale on it.  As noted in the [HorizontalPodAutoscaler
walkthrough](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-multiple-metrics-and-custom-metrics),
there are three different types of metrics that the
HorizontalPodAutoscaler can handle.

In this walkthrough, you've exposed some metrics that can be consumed
using the `Pods` metric type.

Create a description for the HorizontalPodAutoscaler (HPA):

<details>

<summary>sample-app-hpa.yaml</summary>

```yaml
kind: HorizontalPodAutoscaler
apiVersion: autoscaling/v2beta1
metadata:
  name: sample-app
spec:
  scaleTargetRef:
    # point the HPA at the sample application
    # you created above
    apiVersion: apps/v1
    kind: Deployment
    name: sample-app
  # autoscale between 1 and 10 replicas
  minReplicas: 1
  maxReplicas: 10
  metrics:
  # use a "Pods" metric, which takes the average of the
  # given metric across all pods controlled by the autoscaling target
  - type: Pods
    pods:
      # use the metric that you used above: pods/http_requests
      metricName: http_requests
      # target 500 milli-requests per second,
      # which is 1 request every two seconds
      targetAverageValue: 500m
```

</details>

Create the HorizontalPodAutoscaler with

```
$ kubectl create -f sample-app-hpa.yaml
```

Then, like before, make some requests to the sample app's service.  If you
describe the HPA, after the HPA sync interval has elapsed, you should see
the number of pods increase proportionally to the ratio between the actual
requests per second and your target of 1 request every 2 seconds.

You can examine the HPA with

```shell
$ kubectl describe hpa sample-app
```

You should see the HPA's last observed metric value, which should roughly
correspond to the rate of requests that you made.

Next Steps
----------

For more information on how the HPA controller consumes different kinds of
metrics, take a look at the [HPA
walkthrough](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-multiple-metrics-and-custom-metrics).

Also try exposing a non-cumulative metric from your own application, or
scaling on application on a metric provided by another application by
setting different labels or using the `Object` metric source type.

For more information on how metrics are exposed by the Prometheus adapter,
see [config documentation](/docs/config.md), and check the [default
configuration](/deploy/manifests/custom-metrics-config-map.yaml).
