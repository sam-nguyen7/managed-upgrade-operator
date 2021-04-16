#!/bin/bash

export VERSION_MAJOR=0
export VERSION_MINOR=1
export COMMIT_NUMBER=$(git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$$"`..HEAD --count)
export CURRENT_COMMIT=$(git rev-parse --short=7 HEAD)
export OPERATOR_VERSION=$VERSION_MAJOR.$VERSION_MINOR.$COMMIT_NUMBER-$CURRENT_COMMIT

podman build . -f build/Dockerfile -t quay.io/travi/managed-upgrade-operator:v$OPERATOR_VERSION
podman tag quay.io/travi/managed-upgrade-operator:v$OPERATOR_VERSION quay.io/travi/managed-upgrade-operator:latest
