Configuration Walkthroughs
==========================

*If you're looking for reference documentation on configuration, please
read the [configuration reference](/docs/config.md)*

Per-pod HTTP Requests
---------------------

### Background

*The [full walkthrough](/docs/walkthrough.md) sets up the background for
something like this*

Suppose we have some frontend webserver, and we're trying to write a
configuration for the Prometheus adapter so that we can autoscale it based
on the HTTP requests per second that it receives.

Before starting, we've gone and instrumented our frontend server with
a metric, `http_requests_total`.  It is exposed with a single label,
`method`, breaking down the requests by HTTP verb.

We've configured our Prometheus to collect the metric, and it
adds the `kubernetes_namespace` and `kubernetes_pod_name` labels,
representing namespace and pod, respectively.

If we query Prometheus, we see series that look like

```
http_requests_total{method="GET",kubernetes_namespace="production",kubernetes_pod_name="frontend-server-abcd-0123"}
```

### Configuring the adapter

The adapter considers metrics in the following ways:

1. First, it discovers the metrics available (*Discovery*)

2. Then, it figures out which Kubernetes resources each metric is
   associated with (*Association*)

3. Then, it figures out how it should expose them to the custom metrics
   API (*Naming*)

4. Finally, it figures out how it should query Prometheus to get the
   actual numbers (*Querying*)

We need to inform the adapter how it should perform each of these steps
for our metric, `http_requests_total`, so we'll need to add a new
***rule***. Each rule in the adapter encodes these steps.  Let's add a new
one to our configuration:

```yaml
rules:
- {}
```

If we want to find all `http_requests_total` series ourselves in the
Prometheus dashboard, we'd write
`http_requests_total{kubernetes_namespace!="",kubernetes_pod_name!=""}` to
find all `http_requests_total` series that were associated with
a namespace and pod.

We can add this to our rule in the `seriesQuery` field, to tell the
adapter how *discover* the right series itself:

```yaml
rules:
- seriesQuery: 'http_requests_total{kubernetes_namespace!="",kubernetes_pod_name!=""}'
```

Next, we'll need to tell the adapter how to figure out which Kubernetes
resources are associated with the metric.  We've already said that
`kubernetes_namespace` represents the namespace name, and
`kubernetes_pod_name` represents the pod name.  Since these names don't
quite follow a consistent pattern, we use the `overrides` section of the
`resources` field in our rule:

```yaml
rules:
- seriesQuery: 'http_requests_total{kubernetes_namespace!="",kubernetes_pod_name!=""}'
  resources:
    overrides:
      kubernetes_namespace: {resource: "namespace"}
      kubernetes_pod_name: {resource: "pod"}
```

This says that each label represents its corresponding resource. Since the
resources are in the "core" kubernetes API, we don't need to specify
a group.  The adapter will automatically take care of pluralization, so we
can specify either `pod` or `pods`, just the same way as in `kubectl get`.
The resources can be any resource available in your kubernetes cluster, as
long as you've got a corresponding label.

If our labels followed a consistent pattern, like `kubernetes_<resource>`,
we could specify `resources: {template: "kubernetes_<<.Resource>>"}`
instead of specifying an override for each resource.  If you want to see
all resources currently available in your cluster, you can use the
`kubectl api-resources` command (but the list of available resources can
change as you add or remove CRDs or aggregated API servers).  For more
information on resources, see [Kinds, Resources, and
Scopes](https://github.com/kubernetes-sigs/custom-metrics-apiserver/blob/master/docs/getting-started.md#kinds-resources-and-scopes)
in the custom-metrics-apiserver boilerplate guide.

Now, cumulative metrics (like those that end in `_total`) aren't
particularly useful for autoscaling, so we want to convert them to rate
metrics in the API.  We'll call the rate version of our metric
`http_requests_per_second`.  We can use the `name` field to tell the
adapter about that:

```yaml
rules:
- seriesQuery: 'http_requests_total{kubernetes_namespace!="",kubernetes_pod_name!=""}'
  resources:
    overrides:
      kubernetes_namespace: {resource: "namespace"}
      kubernetes_pod_name: {resource: "pod"}
  name:
    matches: "^(.*)_total"
    as: "${1}_per_second"
```

Here, we've said that we should take the name matching
`<something>_total`, and turning it into `<something>_per_second`.

Finally, we need to tell the adapter how to actually query Prometheus to
get some numbers.  Since we want a rate, we might write:
`sum(rate(http_requests_total{kubernetes_namespace="production",kubernetes_pod_name=~"frontend-server-abcd-0123|fronted-server-abcd-4567"}) by (kubernetes_pod_name)`,
which would get us the total requests per second for each pod, summed across verbs.

We can write something similar in the adapter, using the `metricsQuery`
field:

```yaml
rules:
- seriesQuery: 'http_requests_total{kubernetes_namespace!="",kubernetes_pod_name!=""}'
  resources:
    overrides:
      kubernetes_namespace: {resource: "namespace"}
      kubernetes_pod_name: {resource: "pod"}
  name:
    matches: "^(.*)_total"
    as: "${1}_per_second"
  metricsQuery: 'sum(rate(<<.Series>>{<<.LabelMatchers>>}[2m])) by (<<.GroupBy>>)'
```

The adapter will automatically fill in the right series name, label
matchers, and group-by clause, depending on what we put into the API.
Since we're only working with a single metric anyway, we could replace
`<<.Series>>` with `http_requests_total`.

Now, if we run an instance of the Prometheus adapter with this
configuration, we should see discovery information at
`$KUBERNETES/apis/custom.metrics.k8s.io/v1beta1/` of

```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "custom.metrics.k8s.io/v1beta1",
  "resources": [
    {
      "name": "pods/http_requests_total",
      "singularName": "",
      "namespaced": true,
      "kind": "MetricValueList",
      "verbs": ["get"]
    },
    {
      "name": "namespaces/http_requests_total",
      "singularName": "",
      "namespaced": false,
      "kind": "MetricValueList",
      "verbs": ["get"]
    }
  ]
}
```

Notice that we get an entry for both "pods" and "namespaces" -- the
adapter exposes the metric on each resource that we've associated the
metric with (and all namespaced resources must be associated with
a namespace), and will fill in the `<<.GroupBy>>` section with the
appropriate label depending on which we ask for.

We can now connect to
`$KUBERNETES/apis/custom.metrics.k8s.io/v1beta1/namespaces/production/pods/*/http_requests_per_second`,
and we should see

```json
{
  "kind": "MetricValueList",
  "apiVersion": "custom.metrics.k8s.io/v1beta1",
  "metadata": {
    "selfLink": "/apis/custom.metrics.k8s.io/v1beta1/namespaces/production/pods/*/http_requests_per_second",
  },
  "items": [
    {
      "describedObject": {
        "kind": "Pod",
        "name": "frontend-server-abcd-0123",
        "apiVersion": "/__internal",
      },
      "metricName": "http_requests_per_second",
      "timestamp": "2018-08-07T17:45:22Z",
      "value": "16m"
    },
    {
      "describedObject": {
        "kind": "Pod",
        "name": "frontend-server-abcd-4567",
        "apiVersion": "/__internal",
      },
      "metricName": "http_requests_per_second",
      "timestamp": "2018-08-07T17:45:22Z",
      "value": "22m"
    }
  ]
}
```

This says that our server pods are receiving 16 and 22 milli-requests per
second (depending on the pod), which is 0.016 and 0.022 requests per
second, written out as a decimal.  That's about what we'd expect with
little-to-no traffic except for the Prometheus scrape.

If we added some traffic to our pods, we might see `1` or `20` instead of
`16m`, which would be `1` or `20` requests per second.  We might also see
`20500m`, which would mean 20500 milli-requests per second, or 20.5
requests per second in decimal form.
