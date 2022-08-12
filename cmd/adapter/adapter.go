/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	customexternalmetrics "sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"
	"sigs.k8s.io/metrics-server/pkg/api"

	generatedopenapi "sigs.k8s.io/prometheus-adapter/pkg/api/generated/openapi"
	prom "sigs.k8s.io/prometheus-adapter/pkg/client"
	mprom "sigs.k8s.io/prometheus-adapter/pkg/client/metrics"
	adaptercfg "sigs.k8s.io/prometheus-adapter/pkg/config"
	cmprov "sigs.k8s.io/prometheus-adapter/pkg/custom-provider"
	extprov "sigs.k8s.io/prometheus-adapter/pkg/external-provider"
	"sigs.k8s.io/prometheus-adapter/pkg/naming"
	resprov "sigs.k8s.io/prometheus-adapter/pkg/resourceprovider"
)

type PrometheusAdapter struct {
	basecmd.AdapterBase

	// PrometheusURL is the URL describing how to connect to Prometheus.  Query parameters configure connection options.
	PrometheusURL string
	// PrometheusAuthInCluster enables using the auth details from the in-cluster kubeconfig to connect to Prometheus
	PrometheusAuthInCluster bool
	// PrometheusAuthConf is the kubeconfig file that contains auth details used to connect to Prometheus
	PrometheusAuthConf string
	// PrometheusCAFile points to the file containing the ca-root for connecting with Prometheus
	PrometheusCAFile string
	// PrometheusClientTLSCertFile points to the file containing the client TLS cert for connecting with Prometheus
	PrometheusClientTLSCertFile string
	// PrometheusClientTLSKeyFile points to the file containing the client TLS key for connecting with Prometheus
	PrometheusClientTLSKeyFile string
	// PrometheusTokenFile points to the file that contains the bearer token when connecting with Prometheus
	PrometheusTokenFile string
	// PrometheusHeaders is a k=v list of headers to set on requests to PrometheusURL
	PrometheusHeaders []string
	// PrometheusVerb is a verb to set on requests to PrometheusURL
	PrometheusVerb string
	// AdapterConfigFile points to the file containing the metrics discovery configuration.
	AdapterConfigFile string
	// MetricsRelistInterval is the interval at which to relist the set of available metrics
	MetricsRelistInterval time.Duration
	// MetricsMaxAge is the period to query available metrics for
	MetricsMaxAge time.Duration

	metricsConfig *adaptercfg.MetricsDiscoveryConfig
}

func (cmd *PrometheusAdapter) makePromClient() (prom.Client, error) {
	baseURL, err := url.Parse(cmd.PrometheusURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Prometheus URL %q: %v", baseURL, err)
	}

	if cmd.PrometheusVerb != http.MethodGet && cmd.PrometheusVerb != http.MethodPost {
		return nil, fmt.Errorf("unsupported Prometheus HTTP verb %q. use \"GET\" or \"POST\" instead.", cmd.PrometheusVerb)
	}

	var httpClient *http.Client

	if cmd.PrometheusCAFile != "" {
		prometheusCAClient, err := makePrometheusCAClient(cmd.PrometheusCAFile, cmd.PrometheusClientTLSCertFile, cmd.PrometheusClientTLSKeyFile)
		if err != nil {
			return nil, err
		}
		httpClient = prometheusCAClient
		klog.Info("successfully loaded ca from file")
	} else {
		kubeconfigHTTPClient, err := makeKubeconfigHTTPClient(cmd.PrometheusAuthInCluster, cmd.PrometheusAuthConf)
		if err != nil {
			return nil, err
		}
		httpClient = kubeconfigHTTPClient
		klog.Info("successfully using in-cluster auth")
	}

	if cmd.PrometheusTokenFile != "" {
		data, err := ioutil.ReadFile(cmd.PrometheusTokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read prometheus-token-file: %v", err)
		}
		httpClient.Transport = transport.NewBearerAuthRoundTripper(string(data), httpClient.Transport)
	}
	genericPromClient := prom.NewGenericAPIClient(httpClient, baseURL, parseHeaderArgs(cmd.PrometheusHeaders))
	instrumentedGenericPromClient := mprom.InstrumentGenericAPIClient(genericPromClient, baseURL.String())
	return prom.NewClientForAPI(instrumentedGenericPromClient, cmd.PrometheusVerb), nil
}

func (cmd *PrometheusAdapter) addFlags() {
	cmd.Flags().StringVar(&cmd.PrometheusURL, "prometheus-url", cmd.PrometheusURL,
		"URL for connecting to Prometheus.")
	cmd.Flags().BoolVar(&cmd.PrometheusAuthInCluster, "prometheus-auth-incluster", cmd.PrometheusAuthInCluster,
		"use auth details from the in-cluster kubeconfig when connecting to prometheus.")
	cmd.Flags().StringVar(&cmd.PrometheusAuthConf, "prometheus-auth-config", cmd.PrometheusAuthConf,
		"kubeconfig file used to configure auth when connecting to Prometheus.")
	cmd.Flags().StringVar(&cmd.PrometheusCAFile, "prometheus-ca-file", cmd.PrometheusCAFile,
		"Optional CA file to use when connecting with Prometheus")
	cmd.Flags().StringVar(&cmd.PrometheusClientTLSCertFile, "prometheus-client-tls-cert-file", cmd.PrometheusClientTLSCertFile,
		"Optional client TLS cert file to use when connecting with Prometheus, auto-renewal is not supported")
	cmd.Flags().StringVar(&cmd.PrometheusClientTLSKeyFile, "prometheus-client-tls-key-file", cmd.PrometheusClientTLSKeyFile,
		"Optional client TLS key file to use when connecting with Prometheus, auto-renewal is not supported")
	cmd.Flags().StringVar(&cmd.PrometheusTokenFile, "prometheus-token-file", cmd.PrometheusTokenFile,
		"Optional file containing the bearer token to use when connecting with Prometheus")
	cmd.Flags().StringArrayVar(&cmd.PrometheusHeaders, "prometheus-header", cmd.PrometheusHeaders,
		"Optional header to set on requests to prometheus-url. Can be repeated")
	cmd.Flags().StringVar(&cmd.PrometheusVerb, "prometheus-verb", cmd.PrometheusVerb,
		"HTTP verb to set on requests to Prometheus. Possible values: \"GET\", \"POST\"")
	cmd.Flags().StringVar(&cmd.AdapterConfigFile, "config", cmd.AdapterConfigFile,
		"Configuration file containing details of how to transform between Prometheus metrics "+
			"and custom metrics API resources")
	cmd.Flags().DurationVar(&cmd.MetricsRelistInterval, "metrics-relist-interval", cmd.MetricsRelistInterval, ""+
		"interval at which to re-list the set of all available metrics from Prometheus")
	cmd.Flags().DurationVar(&cmd.MetricsMaxAge, "metrics-max-age", cmd.MetricsMaxAge, ""+
		"period for which to query the set of available metrics from Prometheus")
}

func (cmd *PrometheusAdapter) loadConfig() error {
	// load metrics discovery configuration
	if cmd.AdapterConfigFile == "" {
		return fmt.Errorf("no metrics discovery configuration file specified (make sure to use --config)")
	}
	metricsConfig, err := adaptercfg.FromFile(cmd.AdapterConfigFile)
	if err != nil {
		return fmt.Errorf("unable to load metrics discovery configuration: %v", err)
	}

	cmd.metricsConfig = metricsConfig

	return nil
}

func (cmd *PrometheusAdapter) makeProvider(promClient prom.Client, stopCh <-chan struct{}) (provider.CustomMetricsProvider, error) {
	if len(cmd.metricsConfig.Rules) == 0 {
		return nil, nil
	}

	if cmd.MetricsMaxAge < cmd.MetricsRelistInterval {
		return nil, fmt.Errorf("max age must not be less than relist interval")
	}

	// grab the mapper and dynamic client
	mapper, err := cmd.RESTMapper()
	if err != nil {
		return nil, fmt.Errorf("unable to construct RESTMapper: %v", err)
	}
	dynClient, err := cmd.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("unable to construct Kubernetes client: %v", err)
	}

	// extract the namers
	namers, err := naming.NamersFromConfig(cmd.metricsConfig.Rules, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct naming scheme from metrics rules: %v", err)
	}

	// construct the provider and start it
	cmProvider, runner := cmprov.NewPrometheusProvider(mapper, dynClient, promClient, namers, cmd.MetricsRelistInterval, cmd.MetricsMaxAge)
	runner.RunUntil(stopCh)

	return cmProvider, nil
}

func (cmd *PrometheusAdapter) makeExternalProvider(promClient prom.Client, stopCh <-chan struct{}) (provider.ExternalMetricsProvider, error) {
	if len(cmd.metricsConfig.ExternalRules) == 0 {
		return nil, nil
	}

	// grab the mapper
	mapper, err := cmd.RESTMapper()
	if err != nil {
		return nil, fmt.Errorf("unable to construct RESTMapper: %v", err)
	}

	// extract the namers
	namers, err := naming.NamersFromConfig(cmd.metricsConfig.ExternalRules, mapper)
	if err != nil {
		return nil, fmt.Errorf("unable to construct naming scheme from metrics rules: %v", err)
	}

	// construct the provider and start it
	emProvider, runner := extprov.NewExternalPrometheusProvider(promClient, namers, cmd.MetricsRelistInterval, cmd.MetricsMaxAge)
	runner.RunUntil(stopCh)

	return emProvider, nil
}

func (cmd *PrometheusAdapter) addResourceMetricsAPI(promClient prom.Client, stopCh <-chan struct{}) error {
	if cmd.metricsConfig.ResourceRules == nil {
		// bail if we don't have rules for setting things up
		return nil
	}

	mapper, err := cmd.RESTMapper()
	if err != nil {
		return err
	}

	provider, err := resprov.NewProvider(promClient, mapper, cmd.metricsConfig.ResourceRules)
	if err != nil {
		return fmt.Errorf("unable to construct resource metrics API provider: %v", err)
	}

	rest, err := cmd.ClientConfig()
	if err != nil {
		return err
	}

	client, err := metadata.NewForConfig(rest)
	if err != nil {
		return err
	}

	podInformerFactory := metadatainformer.NewFilteredSharedInformerFactory(client, 0, corev1.NamespaceAll, func(options *metav1.ListOptions) {
		options.FieldSelector = "status.phase=Running"
	})
	podInformer := podInformerFactory.ForResource(corev1.SchemeGroupVersion.WithResource("pods"))

	informer, err := cmd.Informers()
	if err != nil {
		return err
	}

	server, err := cmd.Server()
	if err != nil {
		return err
	}

	if err := api.Install(provider, podInformer.Lister(), informer.Core().V1().Nodes().Lister(), server.GenericAPIServer); err != nil {
		return err
	}

	go podInformer.Informer().Run(stopCh)

	return nil
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	// set up flags
	cmd := &PrometheusAdapter{
		PrometheusURL:         "https://localhost",
		PrometheusVerb:        http.MethodGet,
		MetricsRelistInterval: 10 * time.Minute,
	}
	cmd.Name = "prometheus-metrics-adapter"

	cmd.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(api.Scheme, customexternalmetrics.Scheme))
	cmd.OpenAPIConfig.Info.Title = "prometheus-metrics-adapter"
	cmd.OpenAPIConfig.Info.Version = "1.0.0"

	cmd.addFlags()
	// make sure we get klog flags
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	logs.AddGoFlags(local)
	cmd.Flags().AddGoFlagSet(local)
	if err := cmd.Flags().Parse(os.Args); err != nil {
		klog.Fatalf("unable to parse flags: %v", err)
	}

	// if --metrics-max-age is not set, make it equal to --metrics-relist-interval
	if cmd.MetricsMaxAge == 0*time.Second {
		cmd.MetricsMaxAge = cmd.MetricsRelistInterval
	}

	// make the prometheus client
	promClient, err := cmd.makePromClient()
	if err != nil {
		klog.Fatalf("unable to construct Prometheus client: %v", err)
	}

	// load the config
	if err := cmd.loadConfig(); err != nil {
		klog.Fatalf("unable to load metrics discovery config: %v", err)
	}

	// stop channel closed on SIGTERM and SIGINT
	stopCh := genericapiserver.SetupSignalHandler()

	// construct the provider
	cmProvider, err := cmd.makeProvider(promClient, stopCh)
	if err != nil {
		klog.Fatalf("unable to construct custom metrics provider: %v", err)
	}

	// attach the provider to the server, if it's needed
	if cmProvider != nil {
		cmd.WithCustomMetrics(cmProvider)
	}

	// construct the external provider
	emProvider, err := cmd.makeExternalProvider(promClient, stopCh)
	if err != nil {
		klog.Fatalf("unable to construct external metrics provider: %v", err)
	}

	// attach the provider to the server, if it's needed
	if emProvider != nil {
		cmd.WithExternalMetrics(emProvider)
	}

	// attach resource metrics support, if it's needed
	if err := cmd.addResourceMetricsAPI(promClient, stopCh); err != nil {
		klog.Fatalf("unable to install resource metrics API: %v", err)
	}

	// run the server
	if err := cmd.Run(stopCh); err != nil {
		klog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}

// makeKubeconfigHTTPClient constructs an HTTP for connecting with the given auth options.
func makeKubeconfigHTTPClient(inClusterAuth bool, kubeConfigPath string) (*http.Client, error) {
	// make sure we're not trying to use two different sources of auth
	if inClusterAuth && kubeConfigPath != "" {
		return nil, fmt.Errorf("may not use both in-cluster auth and an explicit kubeconfig at the same time")
	}

	// return the default client if we're using no auth
	if !inClusterAuth && kubeConfigPath == "" {
		return http.DefaultClient, nil
	}

	var authConf *rest.Config
	if kubeConfigPath != "" {
		var err error
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
		authConf, err = loader.ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to construct auth configuration from %q for connecting to Prometheus: %v", kubeConfigPath, err)
		}
	} else {
		var err error
		authConf, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to construct in-cluster auth configuration for connecting to Prometheus: %v", err)
		}
	}
	tr, err := rest.TransportFor(authConf)
	if err != nil {
		return nil, fmt.Errorf("unable to construct client transport for connecting to Prometheus: %v", err)
	}
	return &http.Client{Transport: tr}, nil
}

func makePrometheusCAClient(caFilePath string, tlsCertFilePath string, tlsKeyFilePath string) (*http.Client, error) {
	data, err := ioutil.ReadFile(caFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prometheus-ca-file: %v", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no certs found in prometheus-ca-file")
	}

	if (tlsCertFilePath != "") && (tlsKeyFilePath != "") {
		tlsClientCerts, err := tls.LoadX509KeyPair(tlsCertFilePath, tlsKeyFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read TLS key pair: %v", err)
		}
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      pool,
					Certificates: []tls.Certificate{tlsClientCerts},
				},
			},
		}, nil
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: pool,
			},
		},
	}, nil
}

func parseHeaderArgs(args []string) http.Header {
	headers := make(http.Header, len(args))
	for _, h := range args {
		parts := strings.SplitN(h, "=", 2)
		value := ""
		if len(parts) > 1 {
			value = parts[1]
		}
		headers.Add(parts[0], value)
	}
	return headers
}
