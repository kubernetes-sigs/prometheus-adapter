Example Deployment
==================

1. Make sure you've built the included Dockerfile with `TAG=latest make container`. The image should be tagged as `gcr.io/k8s-staging-prometheus-adapter:latest`.

2. Create a secret called `cm-adapter-serving-certs` with two values:
   `serving.crt` and `serving.key`. These are the serving certificates used
   by the adapter for serving HTTPS traffic.  For more information on how to
   generate these certificates, see the [auth concepts
   documentation](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/auth.md)
   in the apiserver-builder repository.
   The kube-prometheus project published two scripts [gencerts.sh](https://github.com/prometheus-operator/kube-prometheus/blob/62fff622e9900fade8aecbd02bc9c557b736ef85/experimental/custom-metrics-api/gencerts.sh)
   and [deploy.sh](https://github.com/prometheus-operator/kube-prometheus/blob/62fff622e9900fade8aecbd02bc9c557b736ef85/experimental/custom-metrics-api/deploy.sh) to create the `cm-adapter-serving-certs` secret.

3. `kubectl create namespace custom-metrics` to ensure that the namespace that we're installing
   the custom metrics adapter in exists.

4. `kubectl create -f manifests/`, modifying the Deployment as necessary to
   point to your Prometheus server, and the ConfigMap to contain your desired
   metrics discovery configuration.
