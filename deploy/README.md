# Example Deployment

1. Make sure you've built the included Dockerfile with `TAG=latest make container`. The image should be tagged as `registry.k8s.io/prometheus-adapter/staging-prometheus-adapter:latest`.

2. `kubectl create namespace monitoring` to ensure that the namespace that we're installing
   the custom metrics adapter in exists.

3. This can be deployed via `kubectl create -f manifests/` or via `kustomize`
   a. if using `kubectl create -f manifests/` modify the Deployment as necessary to point to your Prometheus server, and the ConfigMap to contain your desired metrics discovery configuration.
   b. if using `kustomize` reference the kustomization file located: `http://github.com/kubernetes-sigs/prometheus-adapter/deploy?ref=release-X.XX`
   example kustomization.yaml:

   ```kustomization.yaml
   ---
   apiVersion: kustomize.config.k8s.io/v1beta1
   kind: Kustomization

   resources:
     - https://raw.githubusercontent.com/kubernetes-sigs/prometheus-adapter/release-X.XX/deploy

   patches:
   - path: deployment-patch.yaml
     target:
       kind: Deployment
       version: v1
       name: prometheus-adapter
       namespace: monitoring
   - path: configmap-patch.yaml
     target:
        kind: ConfigMap
        version: v1
        name: adapter-config
        namespace: monitoring
   ```

   ```deployment-patch.yaml
   kind: Deployment
   apiVersion: apps/v1
   metadata:
     name: prometheus-adapter # deployment name to patch
     namespace: monitoring
   spec:
     template:
       spec:
         containers:
         - name: prometheus-adapter # name of container
           args:
           - --cert-dir=/var/run/serving-cert
           - --config=/etc/adapter/config.yaml
           - --metrics-relist-interval=1m
           - --prometheus-url=http://prometheus.monitoring.svc.cluster.local:9090/ # prometheus URL
           - --secure-port=6443
           - --tls-cipher-suites=TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,TLS_RSA_WITH_AES_128_GCM_SHA256,TLS_RSA_WITH_AES_256_GCM_SHA384,TLS_RSA_WITH_AES_128_CBC_SHA,TLS_RSA_WITH_AES_256_CBC_SHA
   ```

   ```configmap-patch.yaml
   kind: ConfigMap
   apiVersion: v1
   metadata:
     labels:
       app.kubernetes.io/component: metrics-adapter
       app.kubernetes.io/name: prometheus-adapter
       app.kubernetes.io/version: 0.12.0
     name: adapter-config
     namespace: monitoring
   data:
     config.yaml: |-
           # your rules would go here
   ```
