#!/bin/bash
set -x
docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD"

if [[ -n $TRAVIS_TAG ]]; then
    make push VERSION=${TRAVIS_TAG}
else
    make push-amd64
fi
