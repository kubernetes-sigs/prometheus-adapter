REGISTRY?=directxman12
IMAGE?=k8s-prometheus-adapter
ARCH?=$(shell go env GOARCH)
ALL_ARCH=amd64 arm arm64 ppc64le s390x
ML_PLATFORMS=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x
OUT_DIR?=$(PWD)/_output

VERSION?=latest
GOIMAGE=golang:1.13
GO111MODULE=on
export GO111MODULE

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

.PHONY: all docker-build push-% push test verify-gofmt gofmt verify build-local-image

all: $(OUT_DIR)/$(ARCH)/adapter

src_deps=$(shell find pkg cmd -type f -name "*.go")
$(OUT_DIR)/%/adapter: $(src_deps)
	CGO_ENABLED=0 GOARCH=$* go build -tags netgo -o $(OUT_DIR)/$*/adapter github.com/directxman12/k8s-prometheus-adapter/cmd/adapter

docker-build: $(OUT_DIR)/Dockerfile
	docker run -it -v $(OUT_DIR):/build -v $(PWD):/go/src/github.com/directxman12/k8s-prometheus-adapter -e GOARCH=$(ARCH) $(GOIMAGE) /bin/bash -c "\
		CGO_ENABLED=0 go build -tags netgo -o /build/$(ARCH)/adapter github.com/directxman12/k8s-prometheus-adapter/cmd/adapter"

	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(VERSION) --build-arg ARCH=$(ARCH) --build-arg BASEIMAGE=$(BASEIMAGE) $(OUT_DIR)

$(OUT_DIR)/Dockerfile: deploy/Dockerfile
	mkdir -p $(OUT_DIR)
	cp deploy/Dockerfile $(OUT_DIR)/Dockerfile

build-local-image: $(OUT_DIR)/Dockerfile $(OUT_DIR)/$(ARCH)/adapter
	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(VERSION) --build-arg ARCH=$(ARCH) --build-arg BASEIMAGE=scratch $(OUT_DIR)

push-%:
	$(MAKE) ARCH=$* docker-build
	docker push $(REGISTRY)/$(IMAGE)-$*:$(VERSION)

push: ./manifest-tool $(addprefix push-,$(ALL_ARCH))
	./manifest-tool push from-args --platforms $(ML_PLATFORMS) --template $(REGISTRY)/$(IMAGE)-ARCH:$(VERSION) --target $(REGISTRY)/$(IMAGE):$(VERSION)

./manifest-tool:
	curl -sSL https://github.com/estesp/manifest-tool/releases/download/v0.5.0/manifest-tool-linux-amd64 > manifest-tool
	chmod +x manifest-tool

vendor:
	go mod tidy
	go mod vendor

test:
	CGO_ENABLED=0 go test ./pkg/...

verify-gofmt:
	./hack/gofmt-all.sh -v

gofmt:
	./hack/gofmt-all.sh

go-mod:
	go mod tidy
	go mod vendor
	go mod verify

verify: verify-gofmt go-mod test
