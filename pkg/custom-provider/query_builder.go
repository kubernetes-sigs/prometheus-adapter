package provider

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

type QueryBuilder interface {
	BuildSelector(seriesName string, groupBy string, groupBySlice []string, queryParts []queryPart) (prom.Selector, error)
}

type queryBuilder struct {
	metricsQueryTemplate *template.Template
}

func NewQueryBuilder(metricsQuery string) (QueryBuilder, error) {
	metricsQueryTemplate, err := template.New("metrics-query").Delims("<<", ">>").Parse(metricsQuery)
	if err != nil {
		return nil, fmt.Errorf("unable to parse metrics query template %q: %v", metricsQuery, err)
	}

	return &queryBuilder{
		metricsQueryTemplate: metricsQueryTemplate,
	}, nil
}

func (n *queryBuilder) BuildSelector(seriesName string, groupBy string, groupBySlice []string, queryParts []queryPart) (prom.Selector, error) {
	//Convert our query parts into the types we need for our template.
	exprs, valuesByName := n.processQueryParts(queryParts)

	args := queryTemplateArgs{
		Series:            seriesName,
		LabelMatchers:     strings.Join(exprs, ","),
		LabelValuesByName: valuesByName,
		GroupBy:           groupBy,
		GroupBySlice:      groupBySlice,
	}

	selector, err := n.createSelectorFromTemplateArgs(args)
	if err != nil {
		return "", err
	}

	return selector, nil
}

func (n *queryBuilder) createSelectorFromTemplateArgs(args queryTemplateArgs) (prom.Selector, error) {
	//Turn our template arguments into a Selector.
	queryBuff := new(bytes.Buffer)
	if err := n.metricsQueryTemplate.Execute(queryBuff, args); err != nil {
		return "", err
	}

	if queryBuff.Len() == 0 {
		return "", fmt.Errorf("empty query produced by metrics query template")
	}

	return prom.Selector(queryBuff.String()), nil
}

func (n *queryBuilder) processQueryParts(queryParts []queryPart) ([]string, map[string][]string) {
	//Contains the expressions that we want to include as part of the query to Prometheus.
	//e.g. "namespace=my-namespace"
	//e.g. "some_label=some-value"
	var exprs []string

	//Contains the list of label values we're targeting, by namespace.
	//e.g. "some_label" => ["value-one", "value-two"]
	valuesByName := map[string][]string{}

	//Convert our query parts into template arguments.
	for _, qPart := range queryParts {
		//Be resilient against bad inputs.
		//We obviously can't generate label filters for these cases.
		if qPart.labelName == "" || len(qPart.values) == 0 {
			continue
		}
		targetValue := qPart.values[0]
		matcher := prom.LabelEq

		if len(qPart.values) > 1 {
			targetValue = strings.Join(qPart.values, "|")
			matcher = prom.LabelMatches
		}

		expression := matcher(qPart.labelName, targetValue)
		exprs = append(exprs, expression)
		valuesByName[qPart.labelName] = qPart.values
	}

	return exprs, valuesByName
}
