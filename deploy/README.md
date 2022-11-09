Example Deployment
==================

1. Make sure you've built the included Dockerfile with `TAG=latest make container`. The image should be tagged as `registry.k8s.io/prometheus-adapter/staging-prometheus-adapter:latest`.

2. `kubectl create namespace monitoring` to ensure that the namespace that we're installing
   the custom metrics adapter in exists.

3. `kubectl create -f manifests/`, modifying the Deployment as necessary to
   point to your Prometheus server, and the ConfigMap to contain your desired
   metrics discovery configuration.
