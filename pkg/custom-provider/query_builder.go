package provider

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/selection"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
)

// QueryBuilder provides functions for generating Prometheus queries.
type QueryBuilder interface {
	BuildSelector(seriesName string, groupBy string, groupBySlice []string, queryParts []queryPart) (prom.Selector, error)
}

type queryBuilder struct {
	metricsQueryTemplate *template.Template
}

// NewQueryBuilder creates a QueryBuilder.
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
	exprs, valuesByName, err := n.processQueryParts(queryParts)

	if err != nil {
		return "", err
	}

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

func (n *queryBuilder) processQueryParts(queryParts []queryPart) ([]string, map[string][]string, error) {
	//We've take the approach here that if we can't perfectly map their query into a Prometheus
	//query that we should abandon the effort completely.
	//The concern is that if we don't get a perfect match on their query parameters, the query result
	//might contain unexpected data that would cause them to take an erroneous action based on the result.

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
		if qPart.labelName == "" {
			return nil, nil, NewLabelNotSpecifiedError()
		}

		if !n.operatorIsSupported(qPart.operator) {
			return nil, nil, NewOperatorNotSupportedByPrometheusError()
		}

		matcher, err := n.selectMatcher(qPart.operator, qPart.values)

		if err != nil {
			return nil, nil, err
		}

		targetValue, err := n.selectTargetValue(qPart.operator, qPart.values)
		if err != nil {
			return nil, nil, err
		}

		expression := matcher(qPart.labelName, targetValue)
		exprs = append(exprs, expression)
		valuesByName[qPart.labelName] = qPart.values
	}

	return exprs, valuesByName, nil
}

func (n *queryBuilder) selectMatcher(operator selection.Operator, values []string) (func(string, string) string, error) {

	numValues := len(values)
	if numValues == 0 {
		switch operator {
		case selection.Exists:
			return prom.LabelMatches, nil
		case selection.DoesNotExist:
			return prom.LabelNotMatches, nil
		case selection.Equals, selection.DoubleEquals, selection.NotEquals, selection.In, selection.NotIn:
			return nil, NewOperatorRequiresValuesError()
		}
	} else if numValues == 1 {
		switch operator {
		case selection.Equals, selection.DoubleEquals:
			return prom.LabelEq, nil
		case selection.NotEquals:
			return prom.LabelNeq, nil
		case selection.In, selection.Exists:
			return prom.LabelMatches, nil
		case selection.DoesNotExist, selection.NotIn:
			return prom.LabelNotMatches, nil
		}
	} else {
		//Since labels can only have one value, providing multiple
		//values results in a regex match, even if that's not what the user
		//asked for.
		switch operator {
		case selection.Equals, selection.DoubleEquals, selection.In, selection.Exists:
			return prom.LabelMatches, nil
		case selection.NotEquals, selection.DoesNotExist, selection.NotIn:
			return prom.LabelNotMatches, nil
		}
	}

	return nil, errors.New("operator not supported by query builder")
}

func (n *queryBuilder) selectTargetValue(operator selection.Operator, values []string) (string, error) {
	numValues := len(values)
	if numValues == 0 {
		switch operator {
		case selection.Exists, selection.DoesNotExist:
			//Regex for any non-empty string.
			//When the operator is LabelNotMatches this will select series without the label
			//or with the label but a value of "".
			//When the operator is LabelMatches this will select series with the label
			//whose value is NOT "".
			return ".+", nil
		case selection.Equals, selection.DoubleEquals, selection.NotEquals, selection.In, selection.NotIn:
			return "", NewOperatorRequiresValuesError()
		}
	} else if numValues == 1 {
		switch operator {
		case selection.Equals, selection.DoubleEquals, selection.NotEquals, selection.In, selection.NotIn:
			//Pass the value through as-is.
			//It's somewhat strange to do this for both the regex and equality
			//operators, but if we do it this way it gives the user a little more control.
			//They might choose to send an "IN" request and give a list of static values
			//or they could send a single value that's a regex, giving them a passthrough
			//for their label selector.
			return values[0], nil
		case selection.Exists, selection.DoesNotExist:
			return "", errors.New("operator does not support values")
		}
	} else {
		switch operator {
		case selection.Equals, selection.DoubleEquals, selection.NotEquals, selection.In, selection.NotIn:
			//Pass the value through as-is.
			//It's somewhat strange to do this for both the regex and equality
			//operators, but if we do it this way it gives the user a little more control.
			//They might choose to send an "IN" request and give a list of static values
			//or they could send a single value that's a regex, giving them a passthrough
			//for their label selector.
			return strings.Join(values, "|"), nil
		case selection.Exists, selection.DoesNotExist:
			return "", NewOperatorDoesNotSupportValuesError()
		}
	}

	return "", errors.New("operator not supported by query builder")
}

func (n *queryBuilder) operatorIsSupported(operator selection.Operator) bool {
	return operator != selection.GreaterThan && operator != selection.LessThan
}
