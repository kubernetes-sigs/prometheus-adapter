#!/bin/bash
docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD"
docker push directxman12/k8s-prometheus-provider:latest
if [[ -n $TRAVIS_TAG ]]; then
    docker push directxman12/k8s-prometheus-provider:$TRAVIS_TAG
fi
