Example Deployment
==================

1. Make sure you've built the included Dockerfile with `make docker-build`. The image should be tagged as `directxman12/k8s-prometheus-adapter:latest`.
   
2. Create a secret called `cm-adapter-serving-certs` with two values:
   `serving.crt` and `serving.key`. For more information on how to
   generate these certificates, see the [auth concepts
   documentation](https://github.com/kubernetes-incubator/apiserver-builder/blob/master/docs/concepts/auth.md)
   in the apiserver-builder repository.

3. `kubectl create namespace custom-metrics` to ensure the namespace we choose to install the custom metrics adapter in.

4. `kubectl create -f manifests/`, modifying as necessary to
   point to your prometheus server.
