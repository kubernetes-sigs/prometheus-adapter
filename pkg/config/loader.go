package config

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/directxman12/k8s-prometheus-adapter/pkg/metrics"

	"gopkg.in/yaml.v2"
)

// FromFile loads the configuration from a particular file.
func FromFile(filename string) (*MetricsDiscoveryConfig, error) {
	file, err := os.Open(filename)
	defer file.Close()
	if err != nil {
		return nil, fmt.Errorf("unable to load metrics discovery config file: %v", err)
	}
	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("unable to load metrics discovery config file: %v", err)
	}
	return FromYAML(contents)
}

// FromYAML loads the configuration from a blob of YAML.
func FromYAML(contents []byte) (*MetricsDiscoveryConfig, error) {
	var cfg MetricsDiscoveryConfig
	if err := yaml.UnmarshalStrict(contents, &cfg); err != nil {
		return nil, fmt.Errorf("unable to parse metrics discovery config: %v", err)
	}
	metrics.Rules.WithLabelValues("normal").Set(float64(len(cfg.Rules)))
	metrics.Rules.WithLabelValues("external").Set(float64(len(cfg.ExternalRules)))
	return &cfg, nil
}
