package nodekeeper

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var IgnoreMasterPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		newNode, ok := e.MetaNew.(*corev1.Node)
		if !ok {
			return false
		}
		nodeLabels := newNode.GetLabels()
		if _, ok := nodeLabels[MasterLabel]; ok {
			log.Info(fmt.Sprintf("Predicate denied reconciling on a Update Event for master node: %s", newNode.Name))
			return false
		}
		return true
	},
	// Create is required to avoid reconciliation at controller initialisation.
	CreateFunc: func(e event.CreateEvent) bool {
		newNodeName := e.Meta.GetName()
		nodeLabels := e.Meta.GetLabels()
		if _, ok := nodeLabels[MasterLabel]; ok {
			log.Info(fmt.Sprintf("Predicate denied reconciling on a Create Event for master node: %s", newNodeName))
			return false
		}
		return true
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		newNodeName := e.Meta.GetName()
		nodeLabels := e.Meta.GetLabels()
		if _, ok := nodeLabels[MasterLabel]; ok {
			log.Info(fmt.Sprintf("Predicate denied reconciling on a Delete Event for master node: %s", newNodeName))
			return false
		}
		return true
	},
	GenericFunc: func(e event.GenericEvent) bool {
		newNodeName := e.Meta.GetName()
		nodeLabels := e.Meta.GetLabels()
		if _, ok := nodeLabels[MasterLabel]; ok {
			log.Info(fmt.Sprintf("Predicate denied reconciling on a Generic Event for master node: %s", newNodeName))
			return false
		}
		return true
	},
}
