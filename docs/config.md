Metrics Discovery and Presentation Configuration
================================================

*If you want a full walkthrough of configuring the adapter for a sample
metric, please read the [configuration
walkthrough](/docs/config-walkthrough.md)*

The adapter determines which metrics to expose, and how to expose them,
through a set of "discovery" rules.  Each rule is executed independently
(so make sure that your rules are mutually exclusive), and specifies each
of the steps the adapter needs to take to expose a metric in the API.

Each rule can be broken down into roughly four parts:

- *Discovery*, which specifies how the adapter should find all Prometheus
  metrics for this rule.

- *Association*, which specifies how the adapter should determine which
  Kubernetes resources a particular metric is associated with.

- *Naming*, which specifies how the adapter should expose the metric in
  the custom metrics API.

- *Querying*, which specifies how a request for a particular metric on one
  or more Kubernetes objects should be turned into a query to Prometheus.

A more comprehensive configuration file can be found in
[sample-config.yaml](sample-config.yaml), but a basic config with one rule
might look like:

```yaml
rules:
# this rule matches cumulative cAdvisor metrics measured in seconds
- seriesQuery: '{__name__=~"^container_.*",container!="POD",namespace!="",pod!=""}'
  resources:
    # skip specifying generic resource<->label mappings, and just
    # attach only pod and namespace resources by mapping label names to group-resources
    overrides:
      namespace: {resource: "namespace"},
      pod: {resource: "pod"},
  # specify that the `container_` and `_seconds_total` suffixes should be removed.
  # this also introduces an implicit filter on metric family names
  name:
    # we use the value of the capture group implicitly as the API name
    # we could also explicitly write `as: "$1"`
    matches: "^container_(.*)_seconds_total$"
  # specify how to construct a query to fetch samples for a given series
  # This is a Go template where the `.Series` and `.LabelMatchers` string values
  # are available, and the delimiters are `<<` and `>>` to avoid conflicts with
  # the prometheus query language
  metricsQuery: "sum(rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[2m])) by (<<.GroupBy>>)"
```

Discovery
---------

Discovery governs the process of finding the metrics that you want to
expose in the custom metrics API.  There are two fields that factor into
discovery: `seriesQuery` and `seriesFilters`.

`seriesQuery` specifies Prometheus series query (as passed to the
`/api/v1/series` endpoint in Prometheus) to use to find some set of
Prometheus series.  The adapter will strip the label values from this
series, and then use the resulting metric-name-label-names combinations
later on.

In many cases, `seriesQuery` will be sufficient to narrow down the list of
Prometheus series.  However, sometimes (especially if two rules might
otherwise overlap), it's useful to do additional filtering on metric
names.  In this case, `seriesFilters` can be used.  After the list of
series is returned from `seriesQuery`, each series has its metric name
filtered through any specified filters.

Filters may be either:

- `is: <regex>`, which matches any series whose name matches the specified
  regex.

- `isNot: <regex>`, which matches any series whose name does not match the
  specified regex.

For example:

```yaml
# match all cAdvisor metrics that aren't measured in seconds
seriesQuery: '{__name__=~"^container_.*_total",container!="POD",namespace!="",pod!=""}'
seriesFilters:
  - isNot: "^container_.*_seconds_total"
```

Association
-----------

Association governs the process of figuring out which Kubernetes resources
a particular metric could be attached to.  The `resources` field controls
this process.

There are two ways to associate resources with a particular metric.  In
both cases, the value of the label becomes the name of the particular
object.

One way is to specify that any label name that matches some particular
pattern refers to some group-resource based on the label name.  This can
be done using the `template` field.   The pattern is specified as a Go
template, with the `Group` and `Resource` fields representing group and
resource. You don't necessarily have to use the `Group` field (in which
case the group is guessed by the system). For instance:

```yaml
# any label `kube_<group>_<resource>` becomes <group>.<resource> in Kubernetes
resources:
  template: "kube_<<.Group>>_<<.Resource>>"
```

The other way is to specify that some particular label represents some
particular Kubernetes resource.  This can be done using the `overrides`
field.  Each override maps a Prometheus label to a Kubernetes
group-resource. For instance:

```yaml
# the microservice label corresponds to the apps.deployment resource
resources:
  overrides:
    microservice: {group: "apps", resource: "deployment"}
```

These two can be combined, so you can specify both a template and some
individual overrides.

The resources mentioned can be any resource available in your kubernetes
cluster, as long as you've got a corresponding label.

Naming
------

Naming governs the process of converting a Prometheus metric name into
a metric in the custom metrics API, and vice versa.  It's controlled by
the `name` field.

Naming is controlled by specifying a pattern to extract an API name from
a Prometheus name, and potentially a transformation on that extracted
value.

The pattern is specified in the `matches` field, and is just a regular
expression.  If not specified, it defaults to `.*`.

The transformation is specified by the `as` field.  You can use any
capture groups defined in the `matches` field.  If the `matches` field
doesn't contain capture groups, the `as` field defaults to `$0`.  If it
contains a single capture group, the `as` field defautls to `$1`.
Otherwise, it's an error not to specify the as field.

For example:

```yaml
# match turn any name <name>_total to <name>_per_second
# e.g. http_requests_total becomes http_requests_per_second
name:
  matches: "^(.*)_total$"
  as: "${1}_per_second"
```

Querying
--------

Querying governs the process of actually fetching values for a particular
metric.  It's controlled by the `metricsQuery` field.

The `metricsQuery` field is a Go template that gets turned into
a Prometheus query, using input from a particular call to the custom
metrics API. A given call to the custom metrics API is distilled down to
a metric name, a group-resource, and one or more objects of that
group-resource.  These get turned into the following fields in the
template:

- `Series`: the metric name
- `LabelMatchers`: a comma-separated list of label matchers matching the
  given objects.  Currently, this is the label for the particular
  group-resource, plus the label for namespace, if the group-resource is
  namespaced.
- `GroupBy`: a comma-separated list of labels to group by.  Currently,
  this contains the group-resource label used in `LabelMatchers`.

For instance, suppose we had a series `http_requests_total` (exposed as
`http_requests_per_second` in the API) with labels `service`, `pod`,
`ingress`, `namespace`, and `verb`. The first four correspond to
Kubernetes resources.  Then, if someone requested the metric
`pods/http_request_per_second` for the pods `pod1` and `pod2` in the
`somens` namespace, we'd have:

- `Series: "http_requests_total"`
- `LabelMatchers: "pod=~\"pod1|pod2",namespace="somens"`
- `GroupBy`: `pod`

Additionally, there are two advanced fields that are "raw" forms of other
fields:

- `LabelValuesByName`: a map mapping the labels and values from the
  `LabelMatchers` field.  The values are pre-joined by `|`
  (for used with the `=~` matcher in Prometheus).
- `GroupBySlice`: the slice form of `GroupBy`.

In general, you'll probably want to use the `Series`, `LabelMatchers`, and
`GroupBy` fields.  The other two are for advanced usage.

The query is expected to return one value for each object requested.  The
adapter will use the labels on the returned series to associate a given
series back to its corresponding object.

For example:

```yaml
# convert cumulative cAdvisor metrics into rates calculated over 2 minutes
metricsQuery: "sum(rate(<<.Series>>{<<.LabelMatchers>>,container!="POD"}[2m])) by (<<.GroupBy>>)"
```
