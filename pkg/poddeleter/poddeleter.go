package poddeleter

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ForceDeletePod deletes a pod received as a formal parameter by
// first removing any associated finalizers before deleting. An error
// is returned if either the removing of the finalizer or the deleting
// itself fails.
func ForceDeletePod(c client.Client, pod *corev1.Pod) error {
	if len(pod.ObjectMeta.GetFinalizers()) != 0 {
		err := removeFinalizers(c, pod)
		if err != nil {
			return err
		}
		// Removing all finalizers will allow the original eviction/deletion
		// call to complete.
		return nil
	}

	err := c.Delete(context.TODO(), pod)
	if err != nil {
		return err
	}
	return nil
}

// removeFinalizers removes all finalizers by updating the formal parameter pod
// object with a empty finalizer slice. After the pod object is updated, the finalizers
// are retrieved again to confirm the expected zero length. An error is returned if the
// update fails or the length of the finalizers post update is non-zero.
func removeFinalizers(c client.Client, p *corev1.Pod) error {
	emptyFinalizer := make([]string, 0)
	p.ObjectMeta.SetFinalizers(emptyFinalizer)

	err := c.Update(context.TODO(), p)
	if err != nil {
		return err
	}

	getFinalizers := p.ObjectMeta.GetFinalizers()
	if len(getFinalizers) != 0 {
		return fmt.Errorf(fmt.Sprintf("Error: Finalizers length non-zero after successful update"))
	}
	return nil
}
