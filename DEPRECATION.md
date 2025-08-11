## Deprecation Notice for Prometheus Adapter

#### Users are advised to follow this document to migrate from (the soon-to-be deprecated) Prometheus Adapter to KEDA.

### Deprecation Rationale

Please refer to the original [deprecation tracker](https://github.com/kubernetes-sigs/prometheus-adapter/issues/701) for the discussion around the deprecation of Prometheus Adapter.

### Deprecation Notice

Everything encompassed by this repository is being deprecated. This includes the Prometheus Adapter itself, all its components, and any related configurations or scripts that are part of this repository. Note that any assets related to Prometheus Adapter that are not part of this repository are not covered by this deprecation notice.

### Deprecation Timeline

The SIG plans to deprecate Prometheus Adapter in the next release cycle, which is expected to be around a couple months from now. After this period, Prometheus Adapter will no longer receive updates or support.

### Deprecation Alternative: KEDA

Below are correlations between the various configuration fields offered by Prometheus Adapter, and their equivalent interpretations using the configuration options offered by KEDA.

However, please keep in mind that:

- The examples below assume you are scaling a single Deployment named `my-app` in namespace `default`, and that your Prometheus instance is reachable at `http://prometheus.monitoring.svc:9090`.
- KEDA's query blocks do not support the templating syntax used in Prometheus Adapter. Instead, you need to write concrete queries that return a single number (instant vector with a single element or a scalar).
  - The queries should be adapted to your environment, and you can use label selectors to narrow down the metrics to the specific pods or namespaces you are interested in (for example, `pod=~"my-app-.*"` and a fixed namespace).
- Unlike Prometheus Adapter, KEDA does not support defining [custom metrics](https://kubernetes.io/docs/reference/external-api/custom-metrics.v1beta2/), and as such, all newly mapped metrics for HPA consumption on KEDA's end are exposed as [external metrics](https://kubernetes.io/docs/reference/external-api/external-metrics.v1beta1/).
  - This is because KEDA's [Prometheus scaler](https://keda.sh/docs/2.17/scalers/prometheus/) exposes all external metrics under the same namespace as the HPA, and does not support dropping namespaces.

The following are exhaustive samples of the configuration fields offered by Prometheus Adapter:

#### `rules`-based configuration

```yaml
rules:
- seriesQuery: '{__name__=~"^container_.*",container!="POD",namespace!="",pod!=""}'
  seriesFilters:
    - is: "^container_.*_total"
  resources:
    template: "kube_<<.Group>>_<<.Resource>>"
    overrides:
      microservice: {group: "apps", resource: "deployment"}
      team: {resource: "namespace"}
  name:
    matches: "^container_(.*)_seconds_total$"
    as: "${1}_per_second"
  metricsQuery: |
    sum(
      rate(
        <<.Series>>{
          <<.LabelMatchers>>,
          pod=~"<<index .LabelValuesByName "pod">>",
          namespace="<<index .LabelValuesByName "namespace">>"
        }[2m]
      )
    ) by (
      {{- range $i, $v := .GroupBySlice }}{{if $i}},{{end}}{{$v}}{{- end}}
    )
```

<details>
  <summary>Corresponding KEDA ScaledObject equivalent</summary>

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-app-scaledobject
  namespace: default
spec:
  # KEDA will create and manage an HPA for this target automatically
  scaleTargetRef:
    # apiVersion: apps/v1 # Optional
    # kind: Deployment    # Optional
    name: my-app          # apps/v1 Deployment (default)  named "my-app"; must be in the *same* namespace
  triggers:
    - name: custom_rate   # Optional, but needed for `scalingModifiers`
      type: prometheus
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090 
        # NOTE: `query` *must* return a single-element vector or a scalar
        query: |
          sum(
            rate(
              container_custom_seconds_total{namespace="default",pod=~"my-app-.*"}[2m]
            )
          )
        # Scale out if the query value exceeds this threshold.
        # If the query returns a value below this threshold, scale in.
        threshold: "1"
        # namespace: example-namespace       # Optional, but needed for multi-tenancy
        # customHeaders: X-Client-Id=cid
        # ignoreNullValues: "true"           # Optional, but needed to ignore empty result sets
        # queryParameters: key-1=value-1
        # unsafeSsl: "false"                 # Optional, but needed to skip TLS verification (self-signed)
        # timeout: 1000                      # Optional, but needed to set a custom HTTP timeout (in ms)
    - name: another_rate
      type: prometheus
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(
            rate(
              container_another_seconds_total{namespace="default",pod=~"my-app-.*"}[2m]
            )
          )
        threshold: "1"
  # Optional: advanced scaling modifiers
  # This section allows you to combine multiple triggers or modify scaling behavior.
  # For example, you can use a formula to combine multiple metrics or adjust scaling behavior.
  # By default, KEDA uses logical OR for multiple triggers, meaning it scales if any trigger is active.
  # See https://keda.sh/docs/2.17/reference/scaledobject-spec/#advanced for details.
  advanced:
    scalingModifiers:
      target: 1
      # See https://github.com/expr-lang/expr for syntax.
      # If the average of the two metrics exceeds 1, scale out; otherwise, scale in.
      formula: "(custom_rate + another_rate) / 2"
    horizontalPodAutoscalerConfig:
      # See https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/#configurable-scaling-behavior for all valid HPA behavior options.
      behavior: 
        scaleDown:
          # Avoid flapping by setting a stabilization window.
          # This prevents rapid scaling in and out.
          stabilizationWindowSeconds: 300
          policies:
            # Allow 100% of the current replicas to be scaled down in 15s
            - type: Percent
              value: 100
              periodSeconds: 15
```
</details>

#### `externalRules`-based configuration

```yaml
externalRules:
- seriesQuery: '{__name__="queue_depth",name!=""}'
  metricsQuery: sum(<<.Series>>{<<.LabelMatchers>>}) by (name)
  resources:
    # Ignore HPA namespace on the metric(s).
    # This allows you to attach the metric to a different namespace than that of the HPA.
    namespaced: false
```


<details>
  <summary>Corresponding KEDA ScaledObject equivalent</summary>

The aforementioned equivalent configuration can similarly be inferred from as follows:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-app-scaledobject
  namespace: default
spec:
  scaleTargetRef:
    name: my-app
  triggers:
    - type: prometheus
      name: queue_depth
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(queue_depth{name!=""})
        # Scale out if the query value exceeds this threshold.
        # If the query returns a value below this threshold, scale in.
        threshold: "100"
        namespace: "queue"  # Optional, but needed to target a specific namespace.
```

</details>

#### `resourceRules`-based configuration

```yaml
resourceRules:
  cpu:
    containerLabel: container
    containerQuery: |
      sum by (<<.GroupBy>>) (
        irate(container_cpu_usage_seconds_total{<<.LabelMatchers>>,container!="",pod!=""}[5m])
      )
    nodeQuery: |
      sum by (<<.GroupBy>>) (
        1 - irate(node_cpu_seconds_total{mode="idle",<<.LabelMatchers>>}[5m])
      )
    resources:
      overrides:
        namespace:
          resource: namespace
        node:
          resource: node
        pod:
          resource: pod
  memory:
    containerLabel: container
    containerQuery: |
      sum by (<<.GroupBy>>) (
        container_memory_working_set_bytes{<<.LabelMatchers>>,container!="",pod!=""}
      )
    nodeQuery: |
      sum by (<<.GroupBy>>) (
        node_memory_MemTotal_bytes{<<.LabelMatchers>>}
        -
        node_memory_MemAvailable_bytes{<<.LabelMatchers>>}
      )
    resources:
      overrides:
        instance:
          resource: node
        namespace:
          resource: namespace
        pod:
          resource: pod
  # Window is the window size reported by the resource metrics API.
  # It should match the value used in your containerQuery and nodeQuery if you use a `rate` function.
  window: 5m
```

<details>
  <summary>Corresponding KEDA ScaledObject equivalent</summary>

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: my-app-combined-metrics
  namespace: default
spec:
  scaleTargetRef:
    name: my-app
  triggers:
    - type: prometheus
      name: container_cpu
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(
            irate(container_cpu_usage_seconds_total{namespace="default",pod=~"my-app-.*",container!="",pod!=""}[5m])
          )
        threshold: "1"
    - type: prometheus
      name: container_memory
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(
            container_memory_working_set_bytes{namespace="default",pod=~"my-app-.*",container!="",pod!=""}
          )
        threshold: "524288000"
    - type: prometheus
      name: node_cpu
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(
            1 - irate(node_cpu_seconds_total{mode="idle"}[5m])
          )
        threshold: "0.7"
    - type: prometheus
      name: node_memory
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        query: |
          sum(
            node_memory_MemTotal_bytes{}
            - node_memory_MemAvailable_bytes{}
          )
        threshold: "10737418240"
  advanced:
    scalingModifiers:
      target: 1
      # Scale out if the average over the thresholds exceeds 1, otherwise scale in.
      formula: "(container_cpu + container_memory/524288000 + node_cpu/0.7 + node_memory/10737418240) / 4"
    horizontalPodAutoscalerConfig:
      behavior:
        scaleDown:
          stabilizationWindowSeconds: 300
          policies:
            - type: Percent
              value: 100
              periodSeconds: 15
```

Note that users looking to build on top of `metrics-server`'s pod and node usage data can use KEDA's [CPU](https://keda.sh/docs/2.17/scalers/cpu/) and [Memory](https://keda.sh/docs/2.17/scalers/memory/) scalers.

These scalers can also be used in conjunction with Prometheus scalers to create a comprehensive scaling strategy.

</details>

