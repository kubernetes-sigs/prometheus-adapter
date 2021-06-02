package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"

	"sigs.k8s.io/prometheus-adapter/cmd/config-gen/utils"
)

func main() {
	var labelPrefix string
	var rateInterval time.Duration

	cmd := &cobra.Command{
		Short: "Generate a config matching the legacy discovery rules",
		Long: `Generate a config that produces the same functionality
as the legacy discovery rules. This includes discovering metrics and associating
resources according to the Kubernetes instrumention conventions and the cAdvisor
conventions, and auto-converting cumulative metrics into rate metrics.`,
		RunE: func(c *cobra.Command, args []string) error {
			cfg := utils.DefaultConfig(rateInterval, labelPrefix)
			enc := yaml.NewEncoder(os.Stdout)
			if err := enc.Encode(cfg); err != nil {
				return err
			}
			return enc.Close()
		},
	}

	cmd.Flags().StringVar(&labelPrefix, "label-prefix", "",
		"Prefix to expect on labels referring to pod resources.  For example, if the prefix is "+
			"'kube_', any series with the 'kube_pod' label would be considered a pod metric")
	cmd.Flags().DurationVar(&rateInterval, "rate-interval", 5*time.Minute,
		"Period of time used to calculate rate metrics from cumulative metrics")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to generate config: %v\n", err)
		os.Exit(1)
	}
}
