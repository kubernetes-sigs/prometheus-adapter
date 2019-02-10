package naming

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime/schema"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// MetricsQuery represents a compiled metrics query for some set of
// series that can be converted into an series of Prometheus expressions to
// be passed to a client.
type MetricsQuery interface {
	// Build constructs Prometheus expressions to represent this query
	// over the given group-resource.  If namespace is empty, the resource
	// is considered to be root-scoped.  extraGroupBy may be used for cases
	// where we need to scope down more specifically than just the group-resource
	// (e.g. container metrics).
	Build(series string, groupRes schema.GroupResource, namespace string, extraGroupBy []string, resourceNames ...string) (prom.Selector, error)
}

// NewMetricsQuery constructs a new MetricsQuery by compiling the given Go template.
// The delimiters on the template are `<<` and `>>`, and it may use the following fields:
// - Series: the series in question
// - LabelMatchers: a pre-stringified form of the label matchers for the resources in the query
// - LabelMatchersByName: the raw map-form of the above matchers
// - GroupBy: the group-by clause to use for the resources in the query (stringified)
// - GroupBySlice: the raw slice form of the above group-by clause
func NewMetricsQuery(queryTemplate string, resourceConverter ResourceConverter) (MetricsQuery, error) {
	templ, err := template.New("metrics-query").Delims("<<", ">>").Parse(queryTemplate)
	if err != nil {
		return nil, fmt.Errorf("unable to parse metrics query template %q: %v", queryTemplate, err)
	}

	return &metricsQuery{
		resConverter: resourceConverter,
		template:     templ,
	}, nil
}

// metricsQuery is a MetricsQuery based on a compiled Go text template.
// with the delimiters as `<<` and `>>`, and the arguments found in
// queryTemplateArgs.
type metricsQuery struct {
	resConverter ResourceConverter
	template     *template.Template
}

// queryTemplateArgs contains the arguments for the template used in metricsQuery.
type queryTemplateArgs struct {
	Series            string
	LabelMatchers     string
	LabelValuesByName map[string][]string
	GroupBy           string
	GroupBySlice      []string
}

func (q *metricsQuery) Build(series string, resource schema.GroupResource, namespace string, extraGroupBy []string, names ...string) (prom.Selector, error) {
	var exprs []string
	valuesByName := map[string][]string{}

	if namespace != "" {
		namespaceLbl, err := q.resConverter.LabelForResource(NsGroupResource)
		if err != nil {
			return "", err
		}
		exprs = append(exprs, prom.LabelEq(string(namespaceLbl), namespace))
		valuesByName[string(namespaceLbl)] = []string{namespace}
	}

	resourceLbl, err := q.resConverter.LabelForResource(resource)
	if err != nil {
		return "", err
	}
	matcher := prom.LabelEq
	targetValue := names[0]
	if len(names) > 1 {
		matcher = prom.LabelMatches
		targetValue = strings.Join(names, "|")
	}
	exprs = append(exprs, matcher(string(resourceLbl), targetValue))
	valuesByName[string(resourceLbl)] = names

	groupBy := make([]string, 0, len(extraGroupBy)+1)
	groupBy = append(groupBy, string(resourceLbl))
	groupBy = append(groupBy, extraGroupBy...)

	args := queryTemplateArgs{
		Series:            series,
		LabelMatchers:     strings.Join(exprs, ","),
		LabelValuesByName: valuesByName,
		GroupBy:           strings.Join(groupBy, ","),
		GroupBySlice:      groupBy,
	}
	queryBuff := new(bytes.Buffer)
	if err := q.template.Execute(queryBuff, args); err != nil {
		return "", err
	}

	if queryBuff.Len() == 0 {
		return "", fmt.Errorf("empty query produced by metrics query template")
	}

	return prom.Selector(queryBuff.String()), nil
}
