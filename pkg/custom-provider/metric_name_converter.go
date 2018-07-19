package provider

import (
	"fmt"
	"regexp"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	"github.com/directxman12/k8s-prometheus-adapter/pkg/config"
)

type MetricNameConverter interface {
	GetMetricNameForSeries(series prom.Series) (string, error)
}

type metricNameConverter struct {
	nameMatches *regexp.Regexp
	nameAs      string
}

func NewMetricNameConverter(mapping config.NameMapping) (MetricNameConverter, error) {
	var nameMatches *regexp.Regexp
	var err error
	if mapping.Matches != "" {
		nameMatches, err = regexp.Compile(mapping.Matches)
		if err != nil {
			return nil, fmt.Errorf("unable to compile series name match expression %q: %v", mapping.Matches, err)
		}
	} else {
		// this will always succeed
		nameMatches = regexp.MustCompile(".*")
	}
	nameAs := mapping.As
	if nameAs == "" {
		// check if we have an obvious default
		subexpNames := nameMatches.SubexpNames()
		if len(subexpNames) == 1 {
			// no capture groups, use the whole thing
			nameAs = "$0"
		} else if len(subexpNames) == 2 {
			// one capture group, use that
			nameAs = "$1"
		} else {
			return nil, fmt.Errorf("must specify an 'as' value for name matcher %q", mapping.Matches)
		}
	}

	return &metricNameConverter{
		nameMatches: nameMatches,
		nameAs:      nameAs,
	}, nil
}

func (c *metricNameConverter) GetMetricNameForSeries(series prom.Series) (string, error) {
	matches := c.nameMatches.FindStringSubmatchIndex(series.Name)
	if matches == nil {
		return "", fmt.Errorf("series name %q did not match expected pattern %q", series.Name, c.nameMatches.String())
	}
	outNameBytes := c.nameMatches.ExpandString(nil, c.nameAs, series.Name, matches)
	return string(outNameBytes), nil
}
