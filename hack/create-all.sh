#!/bin/bash

oc create -f deploy/namespace.yaml
oc project openshift-managed-upgrade-operator
oc create -f deploy/cluster_role.yaml
oc create -f deploy/cluster_role_binding.yaml
oc create -f deploy/service_account.yaml
oc create -f deploy/prometheus_role.yaml
oc create -f deploy/prometheus_rolebinding.yaml
oc create -f deploy/managed-upgrade-operator-config.yaml
oc create -f deploy/crds/upgrade.managed.openshift.io_upgradeconfigs_crd.yaml
oc create -f deploy/operator.yaml
sleep 3
oc create -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
