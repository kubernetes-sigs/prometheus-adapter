# Run in custom metrics namespace
deploy-adapter-local:
	kubectl apply -f manifests/

# Run in custom metrics namespace
delete-adapter-local:
	kubectl delete -f manifests/

namespace:
	kubectl create -f addepar/custom-metrics-namespace.json

raw:
	kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1

shell:
	kubectl run -it --rm --restart=Never alpine --image=alpine sh --limits="cpu=500m,memory=512Mi"
