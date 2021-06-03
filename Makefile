REGISTRY?=directxman12
IMAGE?=k8s-prometheus-adapter
ARCH?=$(shell go env GOARCH)
ALL_ARCH=amd64 arm arm64 ppc64le s390x
ML_PLATFORMS=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x

TAG?=latest
GOIMAGE=golang:1.16

ifeq ($(ARCH),amd64)
	BASEIMAGE?=busybox
endif
ifeq ($(ARCH),arm)
	BASEIMAGE?=armhf/busybox
endif
ifeq ($(ARCH),arm64)
	BASEIMAGE?=aarch64/busybox
endif
ifeq ($(ARCH),ppc64le)
	BASEIMAGE?=ppc64le/busybox
endif
ifeq ($(ARCH),s390x)
	BASEIMAGE?=s390x/busybox
endif

.PHONY: all
all: prometheus-adapter

# Build
# -----

src_deps=$(shell find pkg cmd -type f -name "*.go")
prometheus-adapter: $(src_deps)
	CGO_ENABLED=0 GOARCH=$(ARCH) go build sigs.k8s.io/prometheus-adapter/cmd/adapter

.PHONY: docker-build
docker-build:
	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(TAG) --build-arg ARCH=$(ARCH) --build-arg BASEIMAGE=$(BASEIMAGE) --build-arg GOIMAGE=$(GOIMAGE) .

.PHONY: push-%
push-%:
	$(MAKE) ARCH=$* docker-build
	docker push $(REGISTRY)/$(IMAGE)-$*:$(TAG)

.PHONY: push
push: ./manifest-tool $(addprefix push-,$(ALL_ARCH))
	./manifest-tool push from-args --platforms $(ML_PLATFORMS) --template $(REGISTRY)/$(IMAGE)-ARCH:$(TAG) --target $(REGISTRY)/$(IMAGE):$(TAG)

./manifest-tool:
	curl -sSL https://github.com/estesp/manifest-tool/releases/download/v0.5.0/manifest-tool-linux-amd64 > manifest-tool
	chmod +x manifest-tool

.PHONY: test
test:
	CGO_ENABLED=0 go test ./cmd/... ./pkg/...

.PHONY: verify-gofmt
verify-gofmt:
	./hack/gofmt-all.sh -v

.PHONY: gofmt
gofmt:
	./hack/gofmt-all.sh

.PHONY: verify
verify: verify-gofmt verify-deps verify-generated test

.PHONY: update
update: update-generated

# Dependencies
# ------------

.PHONY: verify-deps
verify-deps:
	go mod verify
	go mod tidy
	@git diff --exit-code -- go.mod go.sum

# Generated
# ---------

generated_files=pkg/api/generated/openapi/zz_generated.openapi.go

.PHONY: verify-generated
verify-generated:
	@git diff --exit-code -- $(generated_files)

.PHONY: update-generated
update-generated:
	go install -mod=readonly k8s.io/kube-openapi/cmd/openapi-gen
	$(GOPATH)/bin/openapi-gen --logtostderr -i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/metrics/pkg/apis/metrics,k8s.io/metrics/pkg/apis/metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 -h ./hack/boilerplate.go.txt -p ./pkg/api/generated/openapi -O zz_generated.openapi -o ./ -r /dev/null
