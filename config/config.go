package config

import (
	"context"

	"github.com/go-logr/logr"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OperatorName string = "managed-upgrade-operator"
)

func GetUpgradeConfigCR(c client.Client, ns string, logger logr.Logger) (*upgradev1alpha1.UpgradeConfig, error) {
	uCList := &upgradev1alpha1.UpgradeConfigList{}

	err := c.List(context.TODO(), uCList, &client.ListOptions{Namespace: ns})
	if err != nil {
		return nil, err
	}

	for _, uC := range uCList.Items {
		return &uC, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: upgradev1alpha1.SchemeGroupVersion.Group, Resource: "UpgradeConfig"}, "UpgradeConfig")

}
