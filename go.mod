module github.com/kubernetes-sigs/prometheus-adapter

go 1.16

require (
	github.com/go-openapi/spec v0.20.3
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20210311094424-0ca2b1909cdc
	github.com/onsi/ginkgo v1.16.2
	github.com/onsi/gomega v1.12.0
	github.com/prometheus/client_golang v1.10.0
	github.com/prometheus/common v0.25.0
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/apiserver v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/component-base v0.21.1
	k8s.io/klog/v2 v2.8.0
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/metrics v0.21.1
	k8s.io/sample-apiserver v0.21.1
	sigs.k8s.io/metrics-server v0.5.0
)

// forced by the inclusion of sigs.k8s.io/metrics-server's use of this in their go.mod
replace k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 => ./localvendor/k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1
