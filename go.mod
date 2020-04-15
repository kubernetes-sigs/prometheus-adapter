module github.com/directxman12/k8s-prometheus-adapter

go 1.13

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20200323093244-5046ce1afe6b
	github.com/onsi/ginkgo v1.10.1
	github.com/onsi/gomega v1.7.0
	github.com/prometheus/client_golang v1.0.0
	github.com/prometheus/common v0.4.1
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.4.0
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	k8s.io/component-base v0.17.3
	k8s.io/klog v1.0.0
	k8s.io/metrics v0.17.3
	sigs.k8s.io/metrics-server v0.3.7
)

replace (
	// forced by the inclusion of sigs.k8s.io/metrics-server's use of this in their go.mod
	k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1 => ./localvendor/k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1
	sigs.k8s.io/metrics-server v0.3.7 => sigs.k8s.io/metrics-server v0.0.0-20200406215547-5fcf6956a533
)
