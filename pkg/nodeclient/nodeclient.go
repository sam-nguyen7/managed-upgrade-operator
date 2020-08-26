// Package nodeclient provides functions to detect if nodes are draining.
package nodeclient

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeClienter interface {
	IsDraining(node *corev1.Node) bool
	GetDrainStartedAtTimestamp(node *corev1.Node) (metav1.Time, error)
	IsTimeToDrain(drainStartedAtTimestamp metav1.Time, drainGracePeriodInMinutes time.Duration) bool
	GetPodsFromNode(c client.Client, node *corev1.Node) (*corev1.PodList, error)
}

type nodeClient struct{}

func NewNodeClient() NodeClienter {
	return &nodeClient{}
}

// IsDraining inspects a type of Node for required boolean and Taints.
// https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#drain
func (nodeC *nodeClient) IsDraining(node *corev1.Node) bool {
	if node.Spec.Unschedulable && len(node.Spec.Taints) > 0 {
		for _, n := range node.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule {
				return true
			}
		}
	}
	return false
}

// GetDrainGetDrainStartedAtTimestamp returns the timestamp of when the draining
// of a node commenced.
func (nodeC *nodeClient) GetDrainStartedAtTimestamp(node *corev1.Node) (metav1.Time, error) {
	var drainStartedAtTimestamp metav1.Time
	for _, n := range node.Spec.Taints {
		if n.Effect == corev1.TaintEffectNoSchedule {
			drainStartedAtTimestamp = *n.TimeAdded
		}
	}

	// Validate a TimeAdded field was set.
	var unsetDrainStartedAtTimestamp metav1.Time
	if drainStartedAtTimestamp == unsetDrainStartedAtTimestamp {
		return drainStartedAtTimestamp, fmt.Errorf(fmt.Sprintf("Node %s is missing desired taint or has no timeAdded field", node.Name))
	}
	return drainStartedAtTimestamp, nil
}

// IsTimeToDrain returns true if its time to drain based on when a Node drain was started,
// the drainGracePeriodInMinutes and the time now.
func (nodeC *nodeClient) IsTimeToDrain(drainStartedAtTimestamp metav1.Time, drainGracePeriodInMinutes time.Duration) bool {
	if metav1.Now().Time.UTC().After(drainStartedAtTimestamp.UTC().Add(drainGracePeriodInMinutes)) {
		return true
	}
	return false
}

// GetPodsFromNode returns a curated PodList for a given node. Daemon sets are removed
// from the list.
func (nodeC *nodeClient) GetPodsFromNode(c client.Client, node *corev1.Node) (*corev1.PodList, error) {
	allPods := &corev1.PodList{}
	curatedPods := &corev1.PodList{}
	nodeMap := make(map[string]string)
	nodeMap["spec.nodeName"] = node.Name
	nodeField := fields.Set(nodeMap)

	listOpts := []client.ListOption{
		client.MatchingFieldsSelector{Selector: nodeField.AsSelector()},
	}
	err := c.List(context.TODO(), allPods, listOpts...)
	if err != nil {
		return curatedPods, err
	}

	// Exclude pods that belong to daemon sets.
	for _, pod := range allPods.Items {
		for _, OwnerRef := range pod.OwnerReferences {
			if OwnerRef.Kind != "DaemonSet" {
				curatedPods.Items = append(curatedPods.Items, pod)
			}
		}
	}
	return curatedPods, nil
}
