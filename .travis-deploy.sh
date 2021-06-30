#!/bin/bash
set -x
docker login -u "$DOCKER_USERNAME" -p "$DOCKER_PASSWORD"

if [[ -n $TRAVIS_TAG ]]; then
    make push
else
    TAG=latest make push
fi
