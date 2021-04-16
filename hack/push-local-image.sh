#!/bin/bash

export REGISTRY_AUTH_FILE=~/Documents/sre/quay/travi-quay-auth.json
export VERSION_MAJOR=0
export VERSION_MINOR=1
export COMMIT_NUMBER=$(git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$$"`..HEAD --count)
export CURRENT_COMMIT=$(git rev-parse --short=7 HEAD)
export OPERATOR_VERSION=$VERSION_MAJOR.$VERSION_MINOR.$COMMIT_NUMBER-$CURRENT_COMMIT

podman push quay.io/travi/managed-upgrade-operator:v$OPERATOR_VERSION
podman push quay.io/travi/managed-upgrade-operator:latest
