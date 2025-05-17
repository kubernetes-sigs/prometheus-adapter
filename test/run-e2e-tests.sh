#!/usr/bin/env bash

# Copyright 2022 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -x
set -o errexit
set -o nounset

# Tool versions
K8S_VERSION=${KUBERNETES_VERSION:-v1.33.1}   # cf https://hub.docker.com/r/kindest/node/tags
KIND_VERSION=${KIND_VERSION:-v0.28.0}        # cf https://github.com/kubernetes-sigs/kind/releases
PROM_OPERATOR_VERSION=${PROM_OPERATOR_VERSION:-v0.82.2} # cf https://github.com/prometheus-operator/prometheus-operator/releases

# Variables; set to empty if unbound/empty
REGISTRY=${REGISTRY:-}
KIND_E2E=${KIND_E2E:-}
SKIP_INSTALL=${SKIP_INSTALL:-}
SKIP_CLEAN_AFTER=${SKIP_CLEAN_AFTER:-}
CLEAN_BEFORE=${CLEAN_BEFORE:-}

# KUBECONFIG - will be overriden if a cluster is deployed with Kind
KUBECONFIG=${KUBECONFIG:-"${HOME}/.kube/config"}

# A temporary directory used by the tests
E2E_DIR="${PWD}/.e2e"

# The namespace where prometheus-adapter is deployed
NAMESPACE="prometheus-adapter-e2e"

if [[ -z "${REGISTRY}" && -z "${KIND_E2E}" ]]; then
    echo -e "Either REGISTRY or KIND_E2E should be set."
    exit 1
fi

function clean {
    if [[ -n "${KIND_E2E}" ]]; then
        kind delete cluster || true
    else
        kubectl delete -f ./deploy/manifests || true
        kubectl delete -f ./test/prometheus-manifests || true
        kubectl delete namespace "${NAMESPACE}" || true
    fi

    rm -rf "${E2E_DIR}"
}

if [[ -n "${CLEAN_BEFORE}" ]]; then
    clean
fi

function on_exit {
    local error_code="$?"

    echo "Obtaining prometheus-adapter pod logs..."
    kubectl logs -l app.kubernetes.io/name=prometheus-adapter -n "${NAMESPACE}" || true

    if [[ -z "${SKIP_CLEAN_AFTER}" ]]; then
        clean
    fi

    test "${error_code}" == 0 && return;
}
trap on_exit EXIT

if [[ -d "${E2E_DIR}" ]]; then
    echo -e "${E2E_DIR} already exists."
    exit 1
fi
mkdir -p "${E2E_DIR}"

if [[ -n "${KIND_E2E}" ]]; then
    # Install kubectl and kind, if we did not set SKIP_INSTALL
    if [[ -z "${SKIP_INSTALL}" ]]; then
        BIN="${E2E_DIR}/bin"
        mkdir -p "${BIN}"
        curl -Lo "${BIN}/kubectl" "https://dl.k8s.io/release/${K8S_VERSION}/bin/linux/amd64/kubectl" && chmod +x "${BIN}/kubectl"
        curl -Lo "${BIN}/kind" "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64" && chmod +x "${BIN}/kind"
        export PATH="${BIN}:${PATH}"
    fi

    kind create cluster --image "kindest/node:${K8S_VERSION}"

    REGISTRY="localhost"

    KUBECONFIG="${E2E_DIR}/kubeconfig"
    kind get kubeconfig > "${KUBECONFIG}"
fi

# Create the test namespace
kubectl create namespace "${NAMESPACE}"

export REGISTRY
IMAGE_NAME="${REGISTRY}/prometheus-adapter-$(go env GOARCH)"
IMAGE_TAG="v$(cat VERSION)"

if [[ -n "${KIND_E2E}" ]]; then
    make container
    kind load docker-image "${IMAGE_NAME}:${IMAGE_TAG}"
else
    make push
fi

# Install prometheus-operator
kubectl apply -f "https://github.com/prometheus-operator/prometheus-operator/releases/download/${PROM_OPERATOR_VERSION}/bundle.yaml" --server-side

# Install and setup prometheus
kubectl apply -f ./test/prometheus-manifests --server-side

# Customize prometheus-adapter manifests
# TODO: use Kustomize or generate manifests from Jsonnet
cp -r ./deploy/manifests "${E2E_DIR}/manifests"
prom_url="http://prometheus.${NAMESPACE}.svc:9090/"
sed -i -e "s|--prometheus-url=.*$|--prometheus-url=${prom_url}|g" "${E2E_DIR}/manifests/deployment.yaml"
sed -i -e "s|image: .*$|image: ${IMAGE_NAME}:${IMAGE_TAG}|g" "${E2E_DIR}/manifests/deployment.yaml"
find "${E2E_DIR}/manifests" -type f -exec sed -i -e "s|namespace: monitoring|namespace: ${NAMESPACE}|g" {} \;

# Deploy prometheus-adapter
kubectl apply -f "${E2E_DIR}/manifests" --server-side

PROJECT_PREFIX="sigs.k8s.io/prometheus-adapter"
export KUBECONFIG
go test "${PROJECT_PREFIX}/test/e2e/" -v -count=1
