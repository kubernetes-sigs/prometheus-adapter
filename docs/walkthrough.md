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
Kubernetes 1.9+), or enabling custom metrics autoscaling support with
disable CPU autoscaling support.

Note that most of the API versions in this walkthrough target Kubernetes
1.9.  It should still work with 1.7 and 1.8, but you might have to change
some minor details.

### Binaries and Images ###

In order to follow this walkthrough, you'll need container images for
Prometheus and the custom metrics adapter.

Both can be found on Dockerhub under `prom/prometheus` and
`directxman12/k8s-prometheus-adapter`, respectively.

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

If you've never deployed Prometheus before, you'll need an appropriate
Prometheus configuration.  There's a extensive sample Prometheus
configuration in the Prometheus repository
[here](https://github.com/prometheus/prometheus/blob/master/documentation/examples/prometheus-kubernetes.yml).
Be sure to also read the Prometheus documentation on [configuring
Prometheus](https://prometheus.io/docs/operating/configuration/).

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

Place your full configuration (it might just be the above) in a file (call
it `prom-cfg.yaml`, for example), and then create a ConfigMap containing
it:

```shell
$ kubectl -n prom create configmap prometheus --from-file=prometheus.yml=prom-cfg.yaml
```

You'll be using this later when you launch Prometheus.

### Launching the Deployment ###

It's generally easiest to launch the adapter and Prometheus as two
containers in the same pod.  You can use a deployment to manage your
adapter and Prometheus instance.

First, we'll create a ServiceAccount for the deployment to run as:

```yaml
$ kubectl -n prom create serviceaccount prom-cm-adapter
```

Start out with a fairly straightforward Prometheus deployment using the
ConfigMap from above, and proceed from there:

<details>

<summary>prom-adapter.deployment.yaml [Prometheus only]</summary>

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prom-cm-adapter
      containers:
      - image: prom/prometheus:v2.2.0-rc.0
        name: prometheus
        args:
        # point prometheus at the configuration that you mount in below
        - --config.file=/etc/prometheus/prometheus.yml
        ports:
        # this port exposes the dashboard and the HTTP API
        - containerPort: 9090
          name: prom-web
          protocol: TCP
        volumeMounts:
        # you'll mount your ConfigMap volume at /etc/prometheus
        - mountPath: /etc/prometheus
          name: prom-config
      volumes:
      # make your configmap available as a volume, so that you can
      # mount in the Prometheus config from earlier
      - name: config-volume
        configMap:
          name: prometheus
```

</details>

Save this file (for example, as `prom-adapter.deployment.yaml`).  Now,
you'll need to modify the deployment to include the adapter.

The adapter has several options, most of which it shares with any other
standard Kubernetes addon API server.  This means that you'll need proper
API server certificates for it to function properly.  To learn more about
which certificates are needed, and what they mean, see [Concepts: Auth and
Certificates](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/auth.md).

Once you've generated the necessary certificates, you'll need to place
them in a secret.  This walkthrough assumes that you've set up your
cluster to automatically inject client certificates and CA certificates
(see the above concepts documentation).  Make sure you've granted the
default service account for your namespace permission to fetch the
authentication CA ConfigMap:

```shell
$ kubectl create rolebinding prom-ext-auth-reader --role="extension-apiserver-authentication-reader" --serviceaccount=prom:prom-cm-adapter
```

Then, store your serving certificates in a secret:

```shell
$ kubectl -n prom create secret tls serving-cm-adapter --cert=/path/to/cm-adapter/serving.crt --key=/path/to/cm-adapter/serving.key
```

Next, you'll need to make sure that the service account used to launch the
Deployment has permission to list resources in the cluster:

```shell
$ kubectl create clusterrole resource-lister --verb=list --resource="*"
$ kubectl create clusterrolebinding cm-adapter-resource-lister --clusterrole=resource-lister -- serviceaccount=prom:prom-cm-adapter
```

Finally, ensure the deployment has all the necessary permissions to
delegate authentication and authorization decisions to the main API
server.  See [Concepts: Auth and
Certificates](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/auth.md)
for more information.

Next, amend the file above to run the adapter as well.  You may need to
modify this part if you wish to inject the needed certificates a different
way.

<details>

<summary>prom-adapter.deployment.yaml [Adapter & Prometheus]</summary>

```yaml
...
spec:
  containers:
  ...
  - image: directxman12/k8s-prometheus-adapter
    name: cm-adapter
    args:
    - --secure-port=6443
    - --logtostderr=true
    # use your serving certs
    - --tls-cert-file=/var/run/serving-certs/tls.crt
    - --tls-private-key-file=/var/run/serving-certs/tls.key
    # Prometheus is running in the same pod, so you can say your Prometheus
    # is at `localhost`
    - --prometheus-url=http://localhost:9090
    # relist available Prometheus metrics every 1m
    - --metrics-relist-interval=1m
    # calculate rates for cumulative metrics over 30s periods.  This should be *at least*
    # as much as your collection interval for Prometheus.
    - --rate-interval=30s
    # re-discover new available resource names every 10m.  You probably
    # won't need to have this be particularly often, but if you add
    # additional addons to your cluster, the adapter will discover there
    # existance at this interval
    - --discovery-interval=10m
    - --v=4
    ports:
    # this port exposes the custom metrics API
    - containerPort: 6443
      name: https
      protocol: TCP
    volumeMounts:
    - mountPath: /var/run/serving-certs
      name: serving-certs
      readOnly: true
  volumes:
  ...
  - name: serving-certs
    secret:
      secretName: serving-cm-adapter
```

</details>

Next, create the deployment and expose it as a service, mapping port 443
to your pod's port 443, on which you've exposed the custom metrics API:

```shell
$ kubectl -n prom create -f prom-adapter.deployment.yaml
$ kubectl -n prom create service clusterip prometheus --tcp=443:6443
```

### Registering the API ###

Now that you have a running deployment of Prometheus and the adapter,
you'll need to register it as providing the
`custom.metrics.k8s.io/v1beta1` API.

For more information on how this works, see [Concepts:
Aggregation](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/aggregation.md).

You'll need to create an API registration record for the
`custom.metrics.k8s.io/v1beta1` API.  In order to do this, you'll need the
base64 encoded version of the CA certificate used to sign the serving
certificates you created above.  If the CA certificate is stored in
`/tmp/ca.crt`, you can get the base64-encoded form like this:

```shell
$ base64 --w 0 < /tmp/ca.crt
```

Take the resulting value, and place it into the following file:

<details>

<summary>cm-registration.yaml</summary>

*Note that apiregistration moved to stable in 1.10, so you'll need to use
the `apiregistration.k8s.io/v1` API version there*.

```yaml
apiVersion: apiregistration.k8s.io/v1beta1
kind: APIService
metadata:
  name: v1beta1.custom.metrics.k8s.io
spec:
  # this tells the aggregator how to verify that your API server is
  # actually who it claims to be
  caBundle: <base-64-value-from-above>
  # these specify which group and version you're registering the API
  # server for
  group: custom.metrics.k8s.io
  version: v1beta1
  # these control how the aggregator prioritizes your registration.
  # it's not particularly relevant in this case.
  groupPriorityMinimum: 1000
  versionPriority: 10
  # finally, this points the aggregator at the service for your
  # API server that you created
  service:
    name: prometheus
    namespace: prom
```

</details>

Register that registration object with the aggregator:

```shell
$ kubectl create -f cm-registration.yaml
```

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
        # based on your Prometheus config above, this tells prometheus
        # to scrape this pod for metrics on port 8080 at "/metrics"
        prometheus.io/scrape: true
        prometheus.io/port: 8080
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - image: luxas/autoscale-demo
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
see the [format documentation](./format.md).
