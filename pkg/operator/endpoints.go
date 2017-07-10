package operator

import (
	"errors"
	"fmt"

	acrt "github.com/appscode/go/runtime"
	"github.com/appscode/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

// Blocks caller. Intended to be called as a Go routine.
func (c *Operator) WatchEndpoints() {
	defer acrt.HandleCrash()

	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return c.KubeClient.CoreV1().Endpoints(apiv1.NamespaceAll).List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.KubeClient.CoreV1().Endpoints(apiv1.NamespaceAll).Watch(metav1.ListOptions{})
		},
	}
	_, ctrl := cache.NewInformer(lw,
		&apiv1.Endpoints{},
		c.SyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if ep, ok := obj.(*apiv1.Endpoints); ok {
					log.Infof("Endpoints %s@%s added", ep.Name, ep.Namespace)

					if !c.ServiceExists(ep) {
						log.Warningf("Skipping Endpoints %s@%s, as it has no matching service", ep.Name, ep.Namespace)
						return
					}
				}
			},
			UpdateFunc: func(old, new interface{}) {
				oldEndpoints, ok := old.(*apiv1.Endpoints)
				if !ok {
					log.Errorln(errors.New("Invalid Endpoints object"))
					return
				}
				newEndpoints, ok := new.(*apiv1.Endpoints)
				if !ok {
					log.Errorln(errors.New("Invalid Endpoints object"))
					return
				}

				if !c.ServiceExists(newEndpoints) {
					log.Warningf("Skipping Endpoints %s@%s, as it has no matching service", newEndpoints.Name, newEndpoints.Namespace)
					return
				}

			},
			DeleteFunc: func(obj interface{}) {
				if ep, ok := obj.(*apiv1.Endpoints); ok {
					log.Infof("Endpoints %s@%s deleted", ep.Name, ep.Namespace)

					if !c.ServiceExists(ep) {
						log.Warningf("Skipping Endpoints %s@%s, as it has no matching service", ep.Name, ep.Namespace)
						return
					}
				}
			},
		},
	)
	ctrl.Run(wait.NeverStop)
}

func (c *Operator) ServiceExists(ep *apiv1.Endpoints) bool {
	// Checking if this endpoint have a service or not. If
	// this do not have a Service we do not want to update our ingress
	_, err := c.KubeClient.CoreV1().Services(ep.Namespace).Get(ep.Name, metav1.GetOptions{})
	return err == nil
}
