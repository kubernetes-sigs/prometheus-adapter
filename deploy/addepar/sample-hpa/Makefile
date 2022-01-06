sample-app:
	kubectl create -n default -f sample-app.deploy.yaml
	kubectl create -n default -f sample-app.service.yaml

delete-sample-app:
	kubectl delete -n default -f sample-app.deploy.yaml
	kubectl delete -n default -f sample-app.service.yaml

monitor:
	kubectl create -n default -f service-monitor.yaml

hpa:
	kubectl delete -f sample-app.hpa.yaml
	kubectl create -f sample-app.hpa.yaml

test:
	kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta1/namespaces/default/pods/*/http_requests?selector=app%3Dsample-app"

get-ip:
	kubectl get service sample-app -o jsonpath='{ .spec.clusterIP }'

busybox:
	# run wget -qO- IP_ADDRESS
	kubectl run -it --rm --restart=Never busybox --image=gcr.io/google-containers/busybox sh

