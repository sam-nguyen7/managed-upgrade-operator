package nodekeeper

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
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
	"github.com/openshift/managed-upgrade-operator/config"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
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
		} else if !yes.IsUpgrading {
			reqLogger.Info("Cluster detected as NOT upgrading. Proceeding.")
			return reconcile.Result{}, nil
		}
	}

	reqLogger.Info("Cluster detected as upgrading. Proceeding.")

	operatorNamespace, err := configmanager.GetOperatorNamespace()
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get nodeKeeperConfig
	cfm := r.configManagerBuilder.New(r.client, operatorNamespace)
	cfg := &nodeKeeperConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Initialise metrics
	metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch the Node instance
	instance := &corev1.Node{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	podDisruptionBudgetAtLimit := false
	pdbList := &policyv1beta1.PodDisruptionBudgetList{}
	errPDB := r.client.List(context.TODO(), pdbList)
	if errPDB != nil {
		return reconcile.Result{}, errPDB
	}
	for _, pdb := range pdbList.Items {
		if pdb.Status.DesiredHealthy == pdb.Status.ExpectedPods {
			podDisruptionBudgetAtLimit = true
		}
	}

	uc, err := config.GetUpgradeConfigCR(r.client, operatorNamespace, reqLogger)
	var drainStarted metav1.Time
	if instance.Spec.Unschedulable && len(instance.Spec.Taints) > 0 {
		for _, n := range instance.Spec.Taints {
			if n.Effect == corev1.TaintEffectNoSchedule {
				drainStarted = *n.TimeAdded
				if drainStarted.Add(cfg.GetNodeDrainDuration()).Before(metav1.Now().Time) && !podDisruptionBudgetAtLimit {
					reqLogger.Info(fmt.Sprintf("The node cannot be drained within %d minutes.", int64(cfg.GetNodeDrainDuration())))
					metricsClient.UpdateMetricNodeDrainFailed(uc.Name)
					return reconcile.Result{}, nil
				}
			}
		}
	}

	metricsClient.ResetMetricNodeDrainFailed(uc.Name)
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
