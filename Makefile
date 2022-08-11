REGISTRY?=gcr.io/k8s-staging-prometheus-adapter
IMAGE=prometheus-adapter
ARCH?=$(shell go env GOARCH)
ALL_ARCH=amd64 arm arm64 ppc64le s390x

VERSION=$(shell cat VERSION)
TAG_PREFIX=v
TAG?=$(TAG_PREFIX)$(VERSION)

GO_VERSION?=1.18.5

.PHONY: all
all: prometheus-adapter

# Build
# -----

SRC_DEPS=$(shell find pkg cmd -type f -name "*.go")

prometheus-adapter: $(SRC_DEPS)
	CGO_ENABLED=0 GOARCH=$(ARCH) go build sigs.k8s.io/prometheus-adapter/cmd/adapter

.PHONY: container
container:
	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(TAG) --build-arg ARCH=$(ARCH) --build-arg GO_VERSION=$(GO_VERSION) .

# Container push
# --------------

PUSH_ARCH_TARGETS=$(addprefix push-,$(ALL_ARCH))

.PHONY: push
push: container
	docker push $(REGISTRY)/$(IMAGE)-$(ARCH):$(TAG)

push-all: $(PUSH_ARCH_TARGETS) push-multi-arch;

.PHONY: $(PUSH_ARCH_TARGETS)
$(PUSH_ARCH_TARGETS): push-%:
	ARCH=$* $(MAKE) push

.PHONY: push-multi-arch
push-multi-arch: export DOCKER_CLI_EXPERIMENTAL = enabled
push-multi-arch:
	docker manifest create --amend $(REGISTRY)/$(IMAGE):$(TAG) $(shell echo $(ALL_ARCH) | sed -e "s~[^ ]*~$(REGISTRY)/$(IMAGE)\-&:$(TAG)~g")
	@for arch in $(ALL_ARCH); do docker manifest annotate --arch $${arch} $(REGISTRY)/$(IMAGE):$(TAG) $(REGISTRY)/$(IMAGE)-$${arch}:$(TAG); done
	docker manifest push --purge $(REGISTRY)/$(IMAGE):$(TAG)

# Test
# ----

.PHONY: test
test:
	CGO_ENABLED=0 go test ./cmd/... ./pkg/...

# Static analysis
# ---------------

.PHONY: verify
verify: verify-gofmt verify-deps verify-generated test

.PHONY: update
update: update-generated

# Format
# ------

.PHONY: verify-gofmt
verify-gofmt:
	./hack/gofmt-all.sh -v

.PHONY: gofmt
gofmt:
	./hack/gofmt-all.sh

# Dependencies
# ------------

.PHONY: verify-deps
verify-deps:
	go mod verify
	go mod tidy
	@git diff --exit-code -- go.mod go.sum

# Generation
# ----------

generated_files=pkg/api/generated/openapi/zz_generated.openapi.go

.PHONY: verify-generated
verify-generated:
	@git diff --exit-code -- $(generated_files)

.PHONY: update-generated
update-generated:
	go install -mod=readonly k8s.io/kube-openapi/cmd/openapi-gen
	$(GOPATH)/bin/openapi-gen --logtostderr -i k8s.io/metrics/pkg/apis/custom_metrics,k8s.io/metrics/pkg/apis/custom_metrics/v1beta1,k8s.io/metrics/pkg/apis/custom_metrics/v1beta2,k8s.io/metrics/pkg/apis/external_metrics,k8s.io/metrics/pkg/apis/external_metrics/v1beta1,k8s.io/metrics/pkg/apis/metrics,k8s.io/metrics/pkg/apis/metrics/v1beta1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version,k8s.io/api/core/v1 -h ./hack/boilerplate.go.txt -p ./pkg/api/generated/openapi -O zz_generated.openapi -o ./ -r /dev/null
