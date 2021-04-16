#!/bin/bash

echo "Building new image"
bash build-local-image.sh

echo "Pushing new image"
bash push-local-image.sh

echo "Deleting configmap, crd and cr"
oc delete -f deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml
oc delete -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
oc delete -f deploy/managed-upgrade-operator-config.yaml

echo "Creating configmap, crd and cr"
oc create -f deploy/managed-upgrade-operator-config.yaml
oc create -f deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml
oc create -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml

echo "Restarting MUO Pod"
oc delete po -l name=managed-upgrade-operator
sleep 10
