/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	prom "github.com/directxman12/k8s-prometheus-adapter/pkg/client"
	mprom "github.com/directxman12/k8s-prometheus-adapter/pkg/client/metrics"
	adaptercfg "github.com/directxman12/k8s-prometheus-adapter/pkg/config"
	cmprov "github.com/directxman12/k8s-prometheus-adapter/pkg/custom-provider"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/cmd/server"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/dynamicmapper"
)

// NewCommandStartPrometheusAdapterServer provides a CLI handler for 'start master' command
func NewCommandStartPrometheusAdapterServer(out, errOut io.Writer, stopCh <-chan struct{}) *cobra.Command {
	baseOpts := server.NewCustomMetricsAdapterServerOptions(out, errOut)
	o := PrometheusAdapterServerOptions{
		CustomMetricsAdapterServerOptions: baseOpts,
		MetricsRelistInterval:             10 * time.Minute,
		RateInterval:                      5 * time.Minute,
		PrometheusURL:                     "https://localhost",
		DiscoveryInterval:                 10 * time.Minute,
	}

	cmd := &cobra.Command{
		Short: "Launch the custom metrics API adapter server",
		Long:  "Launch the custom metrics API adapter server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunCustomMetricsAdapterServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.SecureServing.AddFlags(flags)
	o.Authentication.AddFlags(flags)
	o.Authorization.AddFlags(flags)
	o.Features.AddFlags(flags)

	flags.StringVar(&o.RemoteKubeConfigFile, "lister-kubeconfig", o.RemoteKubeConfigFile, ""+
		"kubeconfig file pointing at the 'core' kubernetes server with enough rights to list "+
		"any described objets")
	flags.DurationVar(&o.MetricsRelistInterval, "metrics-relist-interval", o.MetricsRelistInterval, ""+
		"interval at which to re-list the set of all available metrics from Prometheus")
	flags.DurationVar(&o.RateInterval, "rate-interval", o.RateInterval, ""+
		"period of time used to calculate rate metrics from cumulative metrics")
	flags.DurationVar(&o.DiscoveryInterval, "discovery-interval", o.DiscoveryInterval, ""+
		"interval at which to refresh API discovery information")
	flags.StringVar(&o.PrometheusURL, "prometheus-url", o.PrometheusURL,
		"URL and configuration for connecting to Prometheus.  Query parameters are used to configure the connection")
	flags.StringVar(&o.LabelPrefix, "label-prefix", o.LabelPrefix,
		"Prefix to expect on labels referring to pod resources.  For example, if the prefix is "+
			"'kube_', any series with the 'kube_pod' label would be considered a pod metric")
	flags.StringVar(&o.AdapterConfigFile, "config", o.AdapterConfigFile,
		"Configuration file containing details of how to transform between Prometheus metrics "+
			"and custom metrics API resources")

	flags.MarkDeprecated("label-prefix", "use --config instead")
	flags.MarkDeprecated("discovery-interval", "use --config instead")

	return cmd
}

func (o PrometheusAdapterServerOptions) RunCustomMetricsAdapterServer(stopCh <-chan struct{}) error {
	var metricsConfig *adaptercfg.MetricsDiscoveryConfig
	if o.AdapterConfigFile != "" {
		var err error
		metricsConfig, err = adaptercfg.FromFile(o.AdapterConfigFile)
		if err != nil {
			return fmt.Errorf("unable to load metrics discovery configuration: %v", err)
		}
	} else {
		metricsConfig = adaptercfg.DefaultConfig(o.RateInterval, o.LabelPrefix)
	}

	config, err := o.Config()
	if err != nil {
		return err
	}

	config.GenericConfig.EnableMetrics = true

	var clientConfig *rest.Config
	if len(o.RemoteKubeConfigFile) > 0 {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: o.RemoteKubeConfigFile}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

		clientConfig, err = loader.ClientConfig()
	} else {
		clientConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return fmt.Errorf("unable to construct lister client config to initialize provider: %v", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to construct discovery client for dynamic client: %v", err)
	}

	dynamicMapper, err := dynamicmapper.NewRESTMapper(discoveryClient, apimeta.InterfacesForUnstructured, o.DiscoveryInterval)
	if err != nil {
		return fmt.Errorf("unable to construct dynamic discovery mapper: %v", err)
	}

	clientPool := dynamic.NewClientPool(clientConfig, dynamicMapper, dynamic.LegacyAPIPathResolverFunc)
	if err != nil {
		return fmt.Errorf("unable to construct lister client to initialize provider: %v", err)
	}

	// TODO: actually configure this client (strip query vars, etc)
	baseURL, err := url.Parse(o.PrometheusURL)
	if err != nil {
		return fmt.Errorf("invalid Prometheus URL %q: %v", baseURL, err)
	}
	genericPromClient := prom.NewGenericAPIClient(http.DefaultClient, baseURL)
	instrumentedGenericPromClient := mprom.InstrumentGenericAPIClient(genericPromClient, baseURL.String())
	promClient := prom.NewClientForAPI(instrumentedGenericPromClient)

	namers, err := cmprov.NamersFromConfig(metricsConfig, dynamicMapper)
	if err != nil {
		return fmt.Errorf("unable to construct naming scheme from metrics rules: %v", err)
	}

	cmProvider, runner := cmprov.NewPrometheusProvider(dynamicMapper, clientPool, promClient, namers, o.MetricsRelistInterval)
	runner.RunUntil(stopCh)

	server, err := config.Complete().New("prometheus-custom-metrics-adapter", cmProvider)
	if err != nil {
		return err
	}
	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}

type PrometheusAdapterServerOptions struct {
	*server.CustomMetricsAdapterServerOptions

	// RemoteKubeConfigFile is the config used to list pods from the master API server
	RemoteKubeConfigFile string
	// MetricsRelistInterval is the interval at which to relist the set of available metrics
	MetricsRelistInterval time.Duration
	// RateInterval is the period of time used to calculate rate metrics
	RateInterval time.Duration
	// DiscoveryInterval is the interval at which discovery information is refreshed
	DiscoveryInterval time.Duration
	// PrometheusURL is the URL describing how to connect to Prometheus.  Query parameters configure connection options.
	PrometheusURL string
	// LabelPrefix is the prefix to expect on labels for Kubernetes resources
	// (e.g. if the prefix is "kube_", we'd expect a "kube_pod" label for pod metrics).
	LabelPrefix string
	// AdapterConfigFile points to the file containing the metrics discovery configuration.
	AdapterConfigFile string
}
