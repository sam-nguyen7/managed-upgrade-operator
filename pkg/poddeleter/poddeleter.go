package poddeleter

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ForceDeletePods(c client.Client, pod *corev1.Pod) error {
	err := removeFinalizers(c, pod)
	if err != nil {
		return err
	}
	err = c.Delete(context.TODO(), pod)
	if err != nil {
		return err
	}
	return nil
}

func removeFinalizers(c client.Client, p *corev1.Pod) error {
	emptyFinalizer := make([]string, 0)
	p.ObjectMeta.SetFinalizers(emptyFinalizer)

	err := c.Update(context.TODO(), p)
	if err != nil {
		return err
	}
	return nil
}
