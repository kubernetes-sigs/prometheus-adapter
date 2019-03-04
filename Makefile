REGISTRY?=directxman12
IMAGE?=k8s-prometheus-adapter
ARCH?=amd64
ALL_ARCH=amd64 arm arm64 ppc64le s390x
ML_PLATFORMS=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x
OUT_DIR?=./_output
VENDOR_DOCKERIZED=0
# You need to run "brew install coreutils gnu-sed" to use "macos" below
LOCAL_OS?=linux
SED_BIN?=sed
READLINK_BIN?=readlink
# Get Mozilla CA Cert store in PEM format from curl, https://curl.haxx.se/docs/caextract.html
CA_CERTIFICATES_URL?=https://curl.haxx.se/ca/cacert.pem

VERSION?=latest
GOIMAGE=golang:1.10

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
ifeq ($(LOCAL_OS),macos)
        SED_BIN=gsed
        READLINK_BIN=greadlink
endif

TEMP_DIR:=$(shell $(READLINK_BIN) -f $(shell mktemp -d))

.PHONY: all docker-build push-% push test verify-gofmt gofmt verify download-ca-certificates build-local-image

all: $(OUT_DIR)/$(ARCH)/adapter

src_deps=$(shell find pkg cmd -type f -name "*.go")
$(OUT_DIR)/%/adapter: $(src_deps)
	CGO_ENABLED=0 GOARCH=$* go build -tags netgo -o $(OUT_DIR)/$*/adapter github.com/directxman12/k8s-prometheus-adapter/cmd/adapter
	
docker-build: | download-ca-certificates
	cp deploy/Dockerfile $(TEMP_DIR)
	cd $(TEMP_DIR) && $(SED_BIN) -i "s|BASEIMAGE|$(BASEIMAGE)|g" Dockerfile

	docker run -it -v $(TEMP_DIR):/build -v $(shell pwd):/go/src/github.com/directxman12/k8s-prometheus-adapter -e GOARCH=$(ARCH) $(GOIMAGE) /bin/bash -c "\
		CGO_ENABLED=0 go build -tags netgo -o /build/adapter github.com/directxman12/k8s-prometheus-adapter/cmd/adapter"

	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(VERSION) $(TEMP_DIR)
	rm -rf $(TEMP_DIR)

download-ca-certificates:
	curl -sSL $(CA_CERTIFICATES_URL) -o $(TEMP_DIR)/cacert.pem
	cd $(TEMP_DIR) && curl -sSL $(CA_CERTIFICATES_URL).sha256 | sha256sum --quiet -c -

build-local-image: $(OUT_DIR)/$(ARCH)/adapter | download-ca-certificates
	cp deploy/Dockerfile $(TEMP_DIR)
	cp $(OUT_DIR)/$(ARCH)/adapter $(TEMP_DIR)
	cd $(TEMP_DIR) && $(SED_BIN) -i "s|BASEIMAGE|scratch|g" Dockerfile
	docker build -t $(REGISTRY)/$(IMAGE)-$(ARCH):$(VERSION) $(TEMP_DIR)
	rm -rf $(TEMP_DIR)

push-%:
	$(MAKE) ARCH=$* docker-build
	docker push $(REGISTRY)/$(IMAGE)-$*:$(VERSION)

push: ./manifest-tool $(addprefix push-,$(ALL_ARCH))
	./manifest-tool push from-args --platforms $(ML_PLATFORMS) --template $(REGISTRY)/$(IMAGE)-ARCH:$(VERSION) --target $(REGISTRY)/$(IMAGE):$(VERSION)

./manifest-tool:
	curl -sSL https://github.com/estesp/manifest-tool/releases/download/v0.5.0/manifest-tool-linux-amd64 > manifest-tool
	chmod +x manifest-tool

vendor: Gopkg.lock
ifeq ($(VENDOR_DOCKERIZED),1)
	docker run -it -v $(shell pwd):/go/src/github.com/directxman12/k8s-prometheus-adapter -w /go/src/github.com/directxman12/k8s-prometheus-adapter golang:1.10 /bin/bash -c "\
		curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh \
		&& dep ensure -vendor-only"
else
	dep ensure -vendor-only -v
endif

test:
	CGO_ENABLED=0 go test ./pkg/...

verify-gofmt:
	./hack/gofmt-all.sh -v

gofmt:
	./hack/gofmt-all.sh

verify: verify-gofmt test
