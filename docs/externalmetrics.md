External Metrics
===========

It's possible to configure [Autoscaling on metrics not related to Kubernetes objects](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale-walkthrough/#autoscaling-on-metrics-not-related-to-kubernetes-objects) in Kubernetes. This is done with a special `External Metrics` system. Using external metrics in Kubernetes with the adapter requires you to configure special `external` rules in the configuration.

The configuration for `external` metrics rules is almost identical to the normal `rules`:

```yaml
externalRules:
- seriesQuery: '{__name__="queue_consumer_lag",name!=""}'
  metricsQuery: sum(<<.Series>>{<<.LabelMatchers>>}) by (name)
  resources:
    overrides: { namespace: {resource: "namespace"} }
```

Namespacing
-----------

All Kubernetes Horizontal Pod Autoscaler (HPA) resources are namespaced. And when you create an HPA that
references an external metric the adapter will automatically add a `namespace` label to the `seriesQuery` you have configured.

This is done because the External Merics API Specification *requires* a namespace component in the URL:

```shell
kubectl get --raw "/apis/external.metrics.k8s.io/v1beta1/namespaces/default/queue_consumer_lag"
```

Cross-Namespace or No Namespace Queries
---------------------------------------

A semi-common scenario is to have a `workload` in one namespace that needs to scale based on a metric from a different namespace. This is normally not
possible with `external` rules because the `namespace` label is set to match that of the source `workload`.

However, you can explicitly disable the automatic add of the HPA namepace to the query, and instead opt to not set a namespace at all, or to target a different namespace.

This is done by setting `namespaced: false` in the `resources` section of the `external` rule:

```yaml
# rules: ...

externalRules:
- seriesQuery: '{__name__="queue_depth",name!=""}'
  metricsQuery: sum(<<.Series>>{<<.LabelMatchers>>}) by (name)
  resources:
    namespaced: false
```

Given the `external` rules defined above any `External` metric query for `queue_depth` will simply ignore the source `namespace` of the HPA. This allows you to explicilty not put a namespace into an external query, or to set the namespace to one that might be different from that of the HPA.

```yaml
apiVersion: autoscaling/v1
kind: HorizontalPodAutoscaler
metadata:
  name: external-queue-scaler
  # the HPA and scaleTargetRef must exist in a namespace
  namespace: default
  annotations:
    # The "External" metric below targets a metricName that has namespaced=false
    # and this allows the metric to explicitly query a different
    # namespace than that of the HPA and scaleTargetRef
    autoscaling.alpha.kubernetes.io/metrics: |
      [
        {
            "type": "External",
            "external": {
                "metricName": "queue_depth",
                "metricSelector": {
                    "matchLabels": {
                        "namespace": "queue",
                        "name": "my-sample-queue"
                    }
                },
                "targetAverageValue": "50"
            }
        }
      ]
spec:
  maxReplicas: 5
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
```
