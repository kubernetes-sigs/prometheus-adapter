package config

import (
	"fmt"
	"io/ioutil"
	"os"

	yaml "gopkg.in/yaml.v2"
)

// FromFile loads the configuration from a particular file.
func FromFile(filename string) (*MetricsDiscoveryConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to load metrics discovery config file: %v", err)
	}
	defer file.Close()
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
	return &cfg, nil
}
