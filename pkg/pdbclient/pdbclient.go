package pdbclient

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/config"
	corev1 "k8s.io/api/core/v1"

	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// For clarity, use pdbLabelsType to pass around the named map type.
type pdbLabelsType map[string]string

type PDBClienter interface {
	GetPDBForceDrainTimeout(c client.Client, ns string, logger logr.Logger) (time.Duration, error)
	GetPDBAlertsWithLabels(c client.Client) (bool, map[string]string, error)
	GetPDBLabelPodsFromNode(c client.Client, pdbLabels pdbLabelsType, node *corev1.Node) (*corev1.PodList, error)
}

type pdbClient struct{}

func NewPDBClient() PDBClienter {
	return &pdbClient{}
}

// Get PGetPDBForceDrainTimeout returns the PDBForceDrainTimeout field from the upgradeConfig
// CR in minutes.
func (pdbC *pdbClient) GetPDBForceDrainTimeout(c client.Client, ns string, logger logr.Logger) (time.Duration, error) {
	uC, err := config.GetUpgradeConfigCR(c, ns, logger)
	if err != nil {
		return 0, err
	}
	return time.Duration(uC.Spec.PDBForceDrainTimeout) * time.Minute, nil

}

// GetPDBAlerts retrieves a PodDisruptionBudgetList and checks if DesiredHealthy
// is equal to ExpectedPods which indicates.
func (pdbC *pdbClient) GetPDBAlertsWithLabels(c client.Client) (bool, map[string]string, error) {
	/* use cases
	maxUnavailable = 0
	minAvailable + DesiredHealthy == replica count*/
	PDBPreventingPodDeletion := false
	pdbMatchLabels := make(map[string]string)

	pdbList := &policyv1beta1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), pdbList)
	if err != nil {
		return false, nil, err
	}

	for _, pdb := range pdbList.Items {
		// TODO: handle multiple PDB objects firing
		// PDB protect pod deletion status.
		// https://github.com/kubernetes/kubernetes/blob/master/pkg/registry/core/pod/storage/eviction.go#L288-L289
		if pdb.Status.PodDisruptionsAllowed == 0 {
			PDBPreventingPodDeletion = true
			pdbMatchLabels := pdb.Spec.Selector.MatchLabels
			return PDBPreventingPodDeletion, pdbMatchLabels, nil
		}
	}
	// Return no alerts and no errors.
	return PDBPreventingPodDeletion, pdbMatchLabels, nil
}

// GetPDBLabelPodsFromNode returns a slice of pod names as strings for given pod labels and target node Name.
func (pdbC *pdbClient) GetPDBLabelPodsFromNode(c client.Client, pdbLabels pdbLabelsType, node *corev1.Node) (*corev1.PodList, error) {
	// TODO: we should be able to return target node with FieldsSelector elegantly
	//	nodeMap := make(map[string]string)
	//	nodeMap["spec.nodeName="] = node.Name
	//nodeL := fields.Set(nodeMap)
	//client.MatchingFieldsSelector{Selector: nodeL.AsSelector()},
	foundPods := &corev1.PodList{}
	podList := &corev1.PodList{}
	pdbL := labels.Set(pdbLabels)

	listOpts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: pdbL.AsSelector()},
	}

	err := c.List(context.TODO(), podList, listOpts...)
	if err != nil {
		return foundPods, err
	}

	for _, pod := range podList.Items {
		if pod.Spec.NodeName == node.Name {
			foundPods.Items = append(foundPods.Items, pod)
		}
	}
	return foundPods, nil
}
