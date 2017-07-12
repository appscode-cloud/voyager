package operator

import (
	acrt "github.com/appscode/go/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

// Blocks caller. Intended to be called as a Go routine.
// ref: https://github.com/kubernetes/kubernetes/issues/46736
func (op *Operator) WatchNamespaces() {
	defer acrt.HandleCrash()

	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return op.KubeClient.CoreV1().Namespaces().List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return op.KubeClient.CoreV1().Namespaces().Watch(metav1.ListOptions{})
		},
	}
	_, ctrl := cache.NewInformer(lw,
		&apiv1.Namespace{},
		op.SyncPeriod,
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj interface{}) {
				if ns, ok := obj.(*apiv1.Namespace); ok {
					if resources, err := op.ExtClient.Certificates(ns.Name).List(metav1.ListOptions{}); err == nil {
						for _, resource := range resources.Items {
							op.ExtClient.Certificates(resource.Namespace).Delete(resource.Name)
						}
					}
					if resources, err := op.ExtClient.Ingresses(ns.Name).List(metav1.ListOptions{}); err == nil {
						for _, resource := range resources.Items {
							op.ExtClient.Ingresses(resource.Namespace).Delete(resource.Name)
						}
					}
				}
			},
		},
	)
	ctrl.Run(wait.NeverStop)
}
