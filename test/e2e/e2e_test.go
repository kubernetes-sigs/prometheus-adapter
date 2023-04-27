/*
Copyright 2022 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoring "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	ns                 = "prometheus-adapter-e2e"
	prometheusInstance = "prometheus"
	deployment         = "prometheus-adapter"
)

var (
	client        clientset.Interface
	promOpClient  monitoring.Interface
	metricsClient metrics.Interface
)

func TestMain(m *testing.M) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if len(kubeconfig) == 0 {
		log.Fatal("KUBECONFIG not provided")
	}

	var err error
	client, promOpClient, metricsClient, err = initializeClients(kubeconfig)
	if err != nil {
		log.Fatalf("Cannot create clients: %v", err)
	}

	ctx := context.Background()
	err = waitForPrometheusReady(ctx, ns, prometheusInstance)
	if err != nil {
		log.Fatalf("Prometheus instance 'prometheus' not ready: %v", err)
	}
	err = waitForDeploymentReady(ctx, ns, deployment)
	if err != nil {
		log.Fatalf("Deployment prometheus-adapter not ready: %v", err)
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}

func initializeClients(kubeconfig string) (clientset.Interface, monitoring.Interface, metrics.Interface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error during client configuration with %v", err)
	}

	clientSet, err := clientset.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error during client creation with %v", err)
	}

	promOpClient, err := monitoring.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error during dynamic client creation with %v", err)
	}

	metricsClientSet, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Error during metrics client creation with %v", err)
	}

	return clientSet, promOpClient, metricsClientSet, nil
}

func waitForPrometheusReady(ctx context.Context, namespace string, name string) error {
	return wait.PollImmediateWithContext(ctx, 5*time.Second, 120*time.Second, func(ctx context.Context) (bool, error) {
		prom, err := promOpClient.MonitoringV1().Prometheuses(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		var reconciled, available *monitoringv1.Condition
		for _, condition := range prom.Status.Conditions {
			cond := condition
			if cond.Type == monitoringv1.Reconciled {
				reconciled = &cond
			} else if cond.Type == monitoringv1.Available {
				available = &cond
			}
		}

		if reconciled == nil {
			log.Printf("Prometheus instance '%s': Waiting for reconciliation status...", name)
			return false, nil
		}
		if reconciled.Status != monitoringv1.ConditionTrue {
			log.Printf("Prometheus instance '%s': Reconciiled = %v. Waiting for reconciliation (reason %s, %q)...", name, reconciled.Status, reconciled.Reason, reconciled.Message)
			return false, nil
		}

		specReplicas := *prom.Spec.Replicas
		availableReplicas := prom.Status.AvailableReplicas
		if specReplicas != availableReplicas {
			log.Printf("Prometheus instance '%s': %v/%v pods are ready. Waiting for all pods to be ready...", name, availableReplicas, specReplicas)
			return false, err
		}

		if available == nil {
			log.Printf("Prometheus instance '%s': Waiting for Available status...", name)
			return false, nil
		}
		if available.Status != monitoringv1.ConditionTrue {
			log.Printf("Prometheus instance '%s': Available = %v. Waiting for Available status... (reason %s, %q)", name, available.Status, available.Reason, available.Message)
			return false, nil
		}

		log.Printf("Prometheus instance '%s': Ready.", name)
		return true, nil
	})
}

func waitForDeploymentReady(ctx context.Context, namespace string, name string) error {
	return wait.PollImmediateWithContext(ctx, 5*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		sts, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
			log.Printf("Deployment %s: %v/%v pods are ready.", name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
			return true, nil
		}
		log.Printf("Deployment %s: %v/%v pods are ready. Waiting for all pods to be ready...", name, sts.Status.ReadyReplicas, *sts.Spec.Replicas)
		return false, nil
	})
}

func TestNodeMetrics(t *testing.T) {
	ctx := context.Background()
	var nodeMetrics *metricsv1beta1.NodeMetricsList
	err := wait.PollImmediateWithContext(ctx, 2*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		var err error
		nodeMetrics, err = metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		nonEmptyNodeMetrics := len(nodeMetrics.Items) > 0
		if !nonEmptyNodeMetrics {
			t.Logf("Node metrics empty... Retrying.")
		}
		return nonEmptyNodeMetrics, nil
	})
	require.NoErrorf(t, err, "Node metrics should not be empty")

	for _, nodeMetric := range nodeMetrics.Items {
		positiveMemory := nodeMetric.Usage.Memory().CmpInt64(0)
		assert.Positivef(t, positiveMemory, "Memory usage for node %s is %v, should be > 0", nodeMetric.Name, nodeMetric.Usage.Memory())

		positiveCPU := nodeMetric.Usage.Cpu().CmpInt64(0)
		assert.Positivef(t, positiveCPU, "CPU usage for node %s is %v, should be > 0", nodeMetric.Name, nodeMetric.Usage.Cpu())
	}
}

func TestPodMetrics(t *testing.T) {
	ctx := context.Background()
	var podMetrics *metricsv1beta1.PodMetricsList
	err := wait.PollImmediateWithContext(ctx, 2*time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		var err error
		podMetrics, err = metricsClient.MetricsV1beta1().PodMetricses(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		nonEmptyNodeMetrics := len(podMetrics.Items) > 0
		if !nonEmptyNodeMetrics {
			t.Logf("Pod metrics empty... Retrying.")
		}
		return nonEmptyNodeMetrics, nil
	})
	require.NoErrorf(t, err, "Pod metrics should not be empty")

	for _, pod := range podMetrics.Items {
		for _, containerMetric := range pod.Containers {
			positiveMemory := containerMetric.Usage.Memory().CmpInt64(0)
			assert.Positivef(t, positiveMemory, "Memory usage for pod %s/%s is %v, should be > 0", pod.Name, containerMetric.Name, containerMetric.Usage.Memory())
		}
	}
}
