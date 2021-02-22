module github.com/kubernetes-sigs/prometheus-adapter

go 1.13

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/go-openapi/spec v0.19.8
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20201110135240-8c12d6d92362
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/apiserver v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/component-base v0.20.2
	k8s.io/klog/v2 v2.4.0
	k8s.io/kube-openapi v0.0.0-20210113233702-8566a335510f
	k8s.io/metrics v0.20.2
	k8s.io/sample-apiserver v0.20.2
	sigs.k8s.io/metrics-server v0.4.1-0.20201126131427-ebfc64a74ae4
)

replace (
	// TODO(dgrisonnet): remove once v0.20.3 is released.
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20210121032832-b18087e841ff
	// forced by the inclusion of sigs.k8s.io/metrics-server's use of this in their go.mod
	k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 => ./localvendor/k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1
)
