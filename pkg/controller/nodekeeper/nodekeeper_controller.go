package nodekeeper

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/nodeclient"
	"github.com/openshift/managed-upgrade-operator/pkg/pdbclient"
	"github.com/openshift/managed-upgrade-operator/pkg/poddeleter"
)

var log = logf.Log.WithName("controller_nodekeeper")

// Add creates a new NodeKeeper Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeKeeper{
		client:               mgr.GetClient(),
		configManagerBuilder: configmanager.NewBuilder(),
		machinery:            machinery.NewMachinery(),
		metricsClientBuilder: metrics.NewBuilder(),
		nodeClient:           nodeclient.NewNodeClient(),
		pdbClient:            pdbclient.NewPDBClient(),
		scheme:               mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodekeeper-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Node, status change will not trigger a reconcile
	err = c.Watch(
		&source.Kind{Type: &corev1.Node{}},
		&handler.EnqueueRequestForObject{},
		IgnoreMasterPredicate)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNodeKeeper implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNodeKeeper{}

// ReconcileNodeKeeper reconciles a NodeKeeper object
type ReconcileNodeKeeper struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client               client.Client
	configManagerBuilder configmanager.ConfigManagerBuilder
	machinery            machinery.Machinery
	metricsClientBuilder metrics.MetricsBuilder
	nodeClient           nodeclient.NodeClienter
	pdbClient            pdbclient.PDBClienter
	scheme               *runtime.Scheme
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NodeKeeper")

	// Determine if the cluster is upgrading
	// TODO: This should go main.go so its not called every reconile
	found := fakeUpgradeState()
	if !found {
		yes, err := r.machinery.IsUpgrading(r.client, "worker")
		if err != nil {
			// An error occurred, return it.
			return reconcile.Result{}, err
		} else if !yes {
			reqLogger.Info("Cluster detected as NOT upgrading. Proceeding.")
			return reconcile.Result{}, nil
		}
	}

	reqLogger.Info("Cluster detected as upgrading. Proceeding.")

	//metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	//if err != nil {
	//	return reconcile.Result{}, err
	//}

	// Fetch the Node instance
	instance := &corev1.Node{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Determine if Node is draining.
	if r.nodeClient.IsDraining(instance) {
		reqLogger.Info(fmt.Sprintf("Node %s is identified as draining. Gathering configs for draining policies.", instance.Name))

		// Confirm expected taint and retreive the start time of the drain.
		drainStartedAtTimestamp, err := r.nodeClient.GetDrainStartedAtTimestamp(instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info(fmt.Sprintf("Drain started at %v", drainStartedAtTimestamp))

		// Get operatornamespace of operator as c.Watch is watching Node, a cluster scoped resource.
		// Need this to get upgradeConfig and configmap.
		operatorNamespace, err := configmanager.GetOperatorNamespace()
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Checking for PodDisruptionBudget alerts.")
		pdbAlerts, pdbLabels, err := r.pdbClient.GetPDBAlertsWithLabels(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Declare these vars in this scope for PDB vs normal node drain analysis
		var pdbAlertsOnNode = true
		var pdbPods *corev1.PodList

		if pdbAlerts {
			// Are the PDB pods on target node?
			pdbPods, err = r.pdbClient.GetPDBLabelPodsFromNode(r.client, pdbLabels, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			if len(pdbPods.Items) == 0 {
				reqLogger.Info(fmt.Sprintf("PDB pods not on Node %s.", instance.Name))
				pdbAlertsOnNode = false
			}
		}

		if pdbAlertsOnNode {
			// Execute PDB flow.
			reqLogger.Info(fmt.Sprintf("Found PDB alerts matching %s", pdbLabels))

			pdbNodeDrainGracePeriodInMinutes, err := r.pdbClient.GetPDBForceDrainTimeout(r.client, operatorNamespace, reqLogger)
			if err != nil {
				if errors.IsNotFound(err) {
					// TODO: should send alert here?
					reqLogger.Info("UpgradeConfig not found. No further action.")
					return reconcile.Result{}, nil
				}
				return reconcile.Result{}, err
			}
			reqLogger.Info(fmt.Sprintf("Set pdbNodeDrainGracePeriodInMinutes as %s", pdbNodeDrainGracePeriodInMinutes))

			yes := r.nodeClient.IsTimeToDrain(drainStartedAtTimestamp, pdbNodeDrainGracePeriodInMinutes)
			logDrainTimes(drainStartedAtTimestamp, pdbNodeDrainGracePeriodInMinutes, reqLogger)
			if yes {
				reqLogger.Info(fmt.Sprintf("Node %s ready to drain.", instance.Name))
				for _, pod := range pdbPods.Items {
					err = poddeleter.ForceDeletePod(r.client, &pod)
					if err != nil {
						reqLogger.Info(fmt.Sprintf("Failed deleting PDB pod %s from Node %s", pod.Name, instance.Name))
						return reconcile.Result{}, err
					}
					reqLogger.Info(fmt.Sprintf("Sucessfully deleted PDB pod %s from Node %s", pod.Name, instance.Name))
				}
				return reconcile.Result{}, nil
			}
			reqLogger.Info(fmt.Sprintf("Node %s not ready to drain.", instance.Name))
			reqLogger.Info("time.NOW < DrainAfterTimestamp. Requeuing")
			return reconcile.Result{}, nil
		}

		/* Standard Node drain - No PDB */
		log.Info("Found no PDB alerts. Evaluating Node drain times against SRE NodeDrainGracePeriod only.")

		// Get nodeKeeperConfig
		cfm := r.configManagerBuilder.New(r.client, operatorNamespace)
		cfg := &nodeKeeperConfig{}
		err = cfm.Into(cfg)
		if err != nil {
			return reconcile.Result{}, err
		}
		// Get the node drain related timeouts.
		sreNodeDrainGracePeriodInMinutes := cfg.GetNodeDrainDuration()
		reqLogger.Info(fmt.Sprintf("Set SRE NodeDrainGracePeriod as %s", sreNodeDrainGracePeriodInMinutes))
		yes := r.nodeClient.IsTimeToDrain(drainStartedAtTimestamp, sreNodeDrainGracePeriodInMinutes)
		logDrainTimes(drainStartedAtTimestamp, sreNodeDrainGracePeriodInMinutes, reqLogger)
		if yes {
			// Get all pods on target node and delete them
			reqLogger.Info(fmt.Sprintf("Node %s ready to drain.", instance.Name))
			deletePods, err := r.nodeClient.GetPodsFromNode(r.client, instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			if len(deletePods.Items) == 0 {
				// TODO: Alert for unknown condition?
				return reconcile.Result{}, fmt.Errorf(fmt.Sprintf("Node %s identified as draining but has no pods", instance.Name))
			}
			for _, pod := range deletePods.Items {
				err = poddeleter.ForceDeletePod(r.client, &pod)
				if err != nil {
					return reconcile.Result{}, err
				}
				reqLogger.Info(fmt.Sprintf("Sucessfully deleted pod %s from Node %s", pod.Name, instance.Name))
			}
		}
		reqLogger.Info(fmt.Sprintf("Node %s not ready to drain.", instance.Name))
		reqLogger.Info("time.NOW < DrainAfterTimestamp. Requeuing")
		return reconcile.Result{}, nil
	} else {
		log.Info(fmt.Sprintf("Node %s not tainted. Requeuing.", instance.Name))
	}

	return reconcile.Result{}, nil
}

// logDrainTimes logs times that determine if its time to drain a node.
func logDrainTimes(drainStartedAtTimestamp metav1.Time, drainGracePeriodInMinutes time.Duration, logger logr.Logger) {
	logger.Info(fmt.Sprintf("drainStartedAtTimestamp: %s", drainStartedAtTimestamp.UTC()))
	logger.Info(fmt.Sprintf("drainGracePeriodInMinutes: %v", drainGracePeriodInMinutes.Minutes()))
	drainAfterTimestamp := drainStartedAtTimestamp.Add(drainGracePeriodInMinutes)
	logger.Info(fmt.Sprintf("drainAfterTimestamp: %s", drainAfterTimestamp.UTC()))
	logger.Info(fmt.Sprintf("time.NOW: %s", metav1.Now().Time.UTC()))
}
