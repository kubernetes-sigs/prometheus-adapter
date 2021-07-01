Walkthrough
===========

This walkthrough will go over the basics of setting up the Prometheus
adapter on your cluster and configuring an autoscaler to use application
metrics sourced from the adapter.

Prerequisites
-------------

### Cluster Configuration

Before getting started, ensure that the main components of your
cluster are configured for autoscaling on custom metrics.  As of
Kubernetes 1.7, this requires enabling the aggregation layer on the API
server and configuring the controller manager to use the metrics APIs via
their REST clients.

Detailed instructions can be found in the Kubernetes documentation under
[Horizontal Pod
Autoscaling](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#support-for-custom-metrics).

Make sure that you've properly configured metrics-server (as default in
Kubernetes 1.9+), or enabling custom metrics autoscaling support will
disable CPU autoscaling support.

Note that most of the API versions in this walkthrough target Kubernetes
1.9+.  Note that current versions of the adapter *only* work with
Kubernetes 1.8+.  Version 0.1.0 works with Kubernetes 1.7, but is
significantly different.

### Binaries and Images

In order to follow this walkthrough, you'll need container images for
Prometheus and the custom metrics adapter.

The [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator),
makes it easy to get up and running with Prometheus.  This walkthrough
will assume you're planning on doing that -- if you've deployed it by hand
instead, you'll need to make a few adjustments to the way you expose
metrics to Prometheus.

The adapter has different images for each arch, which can be found at
`gcr.io/k8s-staging-prometheus-adapter/prometheus-adapter-${ARCH}`. For
instance, if you're on an x86_64 machine, use
`gcr.io/k8s-staging-prometheus-adapter/prometheus-adapter-amd64` image.

There is also an official multi arch image available at
`k8s.gcr.io/prometheus-adapter/prometheus-adapter:${VERSION}`.

If you're feeling adventurous, you can build the latest version of
prometheus-adapter by running `make container` or get the latest image from the
staging registry `gcr.io/k8s-staging-prometheus-adapter/prometheus-adapter`.

Special thanks to [@luxas](https://github.com/luxas) for providing the
demo application for this walkthrough.

The Scenario
------------

Suppose that you've written some new web application, and you know it's
the next best thing since sliced bread.  It's ready to unveil to the
world... except you're not sure that just one instance will handle all the
traffic once it goes viral.  Thankfully, you've got Kubernetes.

Deploy your app into your cluster, exposed via a service so that you can
send traffic to it and fetch metrics from it:

<details>

<summary>sample-app.deploy.yaml</summary>

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sample-app
  labels:
    app: sample-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sample-app
  template:
    metadata:
      labels:
        app: sample-app
    spec:
      containers:
      - image: luxas/autoscale-demo:v0.1.2
        name: metrics-provider
        ports:
        - name: http
          containerPort: 8080
```

</details>

<details>

<summary>sample-app.service.yaml</summary>

```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    app: sample-app
  name: sample-app
spec:
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 8080
  selector:
    app: sample-app
  type: ClusterIP
```

</details>

```shell
$ kubectl create -f sample-app.deploy.yaml
$ kubectl create -f sample-app.service.yaml
```

Now, check your app, which exposes metrics and counts the number of
accesses to the metrics page via the `http_requests_total` metric:

```shell
$ curl http://$(kubectl get service sample-app -o jsonpath='{ .spec.clusterIP }')/metrics
```

Notice that each time you access the page, the counter goes up.

Now, you'll want to make sure you can autoscale your application on that
metric, so that you're ready for your launch.  You can use
a HorizontalPodAutoscaler like this to accomplish the autoscaling:

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

If you try creating that now (and take a look at your controller-manager
logs), you'll see that the that the HorizontalPodAutoscaler controller is
attempting to fetch metrics from
`/apis/custom.metrics.k8s.io/v1beta1/namespaces/default/pods/*/http_requests?selector=app%3Dsample-app`,
but right now, nothing's serving that API.

Before you can autoscale your application, you'll need to make sure that
Kubernetes can read the metrics that your application exposes.

Launching Prometheus and the Adapter
------------------------------------

In order to expose metrics beyond CPU and memory to Kubernetes for
autoscaling, you'll need an "adapter" that serves the custom metrics API.
Since you've got Prometheus metrics, it makes sense to use the
Prometheus adapter to serve metrics out of Prometheus.

### Launching Prometheus

First, you'll need to deploy the Prometheus Operator.  Check out the
[quick start
guide](https://github.com/prometheus-operator/prometheus-operator#quickstart)
for the Operator to deploy a copy of Prometheus.

This walkthrough assumes that Prometheus is deployed in the `prom`
namespace. Most of the sample commands and files are namespace-agnostic,
but there are a few commands or pieces of configuration that rely on that
namespace.  If you're using a different namespace, simply substitute that
in for `prom` when it appears.

### Monitoring Your Application

In order to monitor your application, you'll need to set up
a ServiceMonitor pointing at the application.  Assuming you've set up your
Prometheus instance to use ServiceMonitors with the `app: sample-app`
label, create a ServiceMonitor to monitor the app's metrics via the
service:

<details>

<summary>service-monitor.yaml</summary>

```yaml
kind: ServiceMonitor
apiVersion: monitoring.coreos.com/v1
metadata:
  name: sample-app
  labels:
    app: sample-app
spec:
  selector:
    matchLabels:
      app: sample-app
  endpoints:
  - port: http
```

</details>

```shell
$ kubectl create -f service-monitor.yaml
```

Now, you should see your metrics appear in your Prometheus instance.  Look
them up via the dashboard, and make sure they have the `namespace` and
`pod` labels.

### Launching the Adapter

Now that you've got a running copy of Prometheus that's monitoring your
application, you'll need to deploy the adapter, which knows how to
communicate with both Kubernetes and Prometheus, acting as a translator
between the two.

The [deploy/manifests](/deploy/manifests) directory contains the
appropriate files for creating the Kubernetes objects to deploy the
adapter.

See the [deployment README](/deploy/README.md) for more information about
the steps to deploy the adapter.  Note that if you're deploying on
a non-x86_64 (amd64) platform, you'll need to change the `image` field in
the Deployment to be the appropriate image for your platform.

The default adapter configuration should work for this walkthrough and
a standard Prometheus Operator configuration, but if you've got custom
relabelling rules, or your labels above weren't exactly `namespace` and
`pod`, you may need to edit the configuration in the ConfigMap. The
[configuration walkthrough](/docs/config-walkthrough.md) provides an
overview of how configuration works.

### The Registered API

As part of the creation of the adapter Deployment and associated objects
(performed above), we registered the API with the API aggregator (part of
the main Kubernetes API server).

The API is registered as `custom.metrics.k8s.io/v1beta1`, and you can find
more information about aggregation at [Concepts:
Aggregation](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/aggregation.md).

### Double-Checking Your Work

With that all set, your custom metrics API should show up in discovery.

Try fetching the discovery information for it:

```shell
$ kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1
```

Since you've set up Prometheus to collect your app's metrics, you should
see a `pods/http_request` resource show up.  This represents the
`http_requests_total` metric, converted into a rate, aggregated to have
one datapoint per pod.  Notice that this translates to the same API that
our HorizontalPodAutoscaler was trying to use above.

You can check the value of the metric using `kubectl get --raw`, which
sends a raw GET request to the Kubernetes API server, automatically
injecting auth information:

```shell
$ kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta1/namespaces/default/pods/*/http_requests?selector=app%3Dsample-app"
```

Because of the adapter's configuration, the cumulative metric
`http_requests_total` has been converted into a rate metric,
`pods/http_requests`, which measures requests per second over a 2 minute
interval. The value should currently be close to zero, since there's no
traffic to your app, except for the regular metrics collection from
Prometheus.

Try generating some traffic using cURL a few times, like before:

```shell
$ curl http://$(kubectl get service sample-app -o jsonpath='{ .spec.clusterIP }')/metrics
```

Now, if you fetch the metrics again, you should see an increase in the
value.  If you leave it alone for a bit, the value should go back down
again.

### Quantity Values

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
resources, you might need to modify your [adapter
configuration](/docs/config.md), as mentioned above.  Check your labels
via the Prometheus dashboard, and then modify the configuration
appropriately.

As noted in the main [README](/README.md), you'll need to also make sure
your metrics relist interval is at least your Prometheus scrape interval.
If it's less that that, you'll see metrics periodically appear and
disappear from the adapter.

Autoscaling
-----------

Now that you finally have the metrics API set up, your
HorizontalPodAutoscaler should be able to fetch the appropriate metric,
and make decisions based on it.

If you didn't create the HorizontalPodAutoscaler above, create it now:

```shell
$ kubectl create -f sample-app-hpa.yaml
```

Wait a little bit, and then examine the HPA:

```shell
$ kubectl describe hpa sample-app
```

You should see that it succesfully fetched the metric, but it hasn't tried
to scale, since there's not traffic.

Since your app is going to need to scale in response to traffic, generate
some via cURL like above:

```shell
$ curl http://$(kubectl get service sample-app -o jsonpath='{ .spec.clusterIP }')/metrics
```

Recall from the configuration at the start that you configured your HPA to
have each replica handle 500 milli-requests, or 1 request every two
seconds (ok, so *maybe* you still have some performance issues to work out
before your beta period ends).  Thus, if you generate a few requests, you
should see the HPA scale up your app relatively quickly.

If you describe the HPA again, you should see that the last observed
metric value roughly corresponds to your rate of requests, and that the
HPA has recently scaled your app.

Now that you've got your app autoscaling on the HTTP requests, you're all
ready to launch! If you leave the app alone for a while, the HPA should
scale it back down, so you can save precious budget for the launch party.

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
