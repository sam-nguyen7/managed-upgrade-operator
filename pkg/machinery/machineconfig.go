package machinery

import (
	"context"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsUpgrading determines if machines are currently upgrading by comparing
// MachineCount and UpdatedMachineCount
func (m *machinery) IsUpgrading(c client.Client, nodeType string) (bool, error) {
	configPool := &machineconfigapi.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: nodeType}, configPool)
	if err != nil {
		return false, err
	}
	// Machines are upgrading
	if configPool.Status.MachineCount != configPool.Status.UpdatedMachineCount {
		return true, nil
	}

	// Machines are not upgrading
	return false, nil
}
