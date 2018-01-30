package operator

import (
	"context"

	etx "github.com/appscode/go/context"
	"github.com/appscode/go/log"
	"github.com/appscode/kutil/meta"
	api "github.com/appscode/voyager/apis/voyager/v1beta1"
	"github.com/appscode/voyager/listers/voyager/voyager"
	"github.com/appscode/voyager/pkg/eventer"
	"github.com/appscode/voyager/pkg/ingress"
	"github.com/golang/glog"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rt "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Blocks caller. Intended to be called as a Go routine.
func (op *Operator) initIngressCRDWatcher() {
	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (rt.Object, error) {
			return op.VoyagerClient.Ingresses(op.Opt.WatchNamespace()).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return op.VoyagerClient.Ingresses(op.Opt.WatchNamespace()).Watch(options)
		},
	}

	// create the workqueue
	op.engQueue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "engress")

	op.engIndexer, op.engInformer = cache.NewIndexerInformer(lw, &api.Ingress{}, op.Opt.ResyncPeriod, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			engress, ok := obj.(*api.Ingress)
			if !ok {
				log.Errorln("Invalid Ingress object")
				return
			}
			engress.Migrate()
			if !engress.ShouldHandleIngress(op.Opt.IngressClass) {
				log.Infof("%s %s@%s does not match ingress class", engress.APISchema(), engress.Name, engress.Namespace)
				return
			}
			if err := engress.IsValid(op.Opt.CloudProvider); err != nil {
				op.recorder.Eventf(
					engress.ObjectReference(),
					core.EventTypeWarning,
					eventer.EventReasonIngressInvalid,
					"Reason: %s",
					err.Error(),
				)
				return
			}
			if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
				op.engQueue.Add(key)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			oldEngress, ok := old.(*api.Ingress)
			if !ok {
				log.Errorln("Invalid Ingress object")
				return
			}
			oldEngress.Migrate()
			newEngress, ok := new.(*api.Ingress)
			if !ok {
				log.Errorln("Invalid Ingress object")
				return
			}
			newEngress.Migrate()

			if changed, _ := oldEngress.HasChanged(*newEngress); !changed {
				return
			}
			diff := meta.Diff(oldEngress, newEngress)
			log.Infof("%s %s@%s has changed. Diff: %s", newEngress.APISchema(), newEngress.Name, newEngress.Namespace, diff)

			if err := newEngress.IsValid(op.Opt.CloudProvider); err != nil {
				op.recorder.Eventf(
					newEngress.ObjectReference(),
					core.EventTypeWarning,
					eventer.EventReasonIngressInvalid,
					"Reason: %s",
					err.Error(),
				)
				return
			}
			if key, err := cache.MetaNamespaceKeyFunc(new); err == nil {
				op.engQueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if engress, ok := obj.(*api.Ingress); ok {
				engress.Migrate()
				if !engress.ShouldHandleIngress(op.Opt.IngressClass) {
					log.Infof("%s %s@%s does not match ingress class", engress.APISchema(), engress.Name, engress.Namespace)
					return
				}
				if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
					op.engQueue.Add(key)
				}
			}
		},
	}, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

	op.engLister = voyager.NewIngressLister(op.engIndexer)
}

func (op *Operator) runEngressWatcher() {
	for op.processNextEngress() {
	}
}

func (op *Operator) processNextEngress() bool {
	// Wait until there is a new item in the working queue
	key, quit := op.engQueue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two objects with the same key are never processed in
	// parallel.
	defer op.engQueue.Done(key)

	// Invoke the method containing the business logic
	err := op.runEngressInjector(key.(string))
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		op.engQueue.Forget(key)
		return true
	}
	log.Errorf("Failed to process engress %v. Reason: %s", key, err)

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if op.engQueue.NumRequeues(key) < op.Opt.MaxNumRequeues {
		glog.Infof("Error syncing engress %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		op.engQueue.AddRateLimited(key)
		return true
	}

	op.engQueue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	glog.Infof("Dropping engress %q out of the queue: %v", key, err)
	return true
}

func (op *Operator) runEngressInjector(key string) error {
	obj, exists, err := op.engIndexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	if !exists {
		glog.Warningf("Engress %s does not exist anymore\n", key)
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			return err
		}
		engress := &api.Ingress{ // fake engress object
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		op.DeleteEngress(etx.Background(), engress)
	} else {
		glog.Infof("Sync/Add/Update for engress %s\n", key)
		engress := obj.(*api.Ingress)
		engress.Migrate()
		op.AddEngress(etx.Background(), engress)
	}
	return nil
}

func (op *Operator) AddEngress(ctx context.Context, engress *api.Ingress) {
	ctrl := ingress.NewController(ctx, op.KubeClient, op.CRDClient, op.VoyagerClient, op.PromClient, op.ServiceLister, op.EndpointsLister, op.Opt, engress)
	ctrl.Create()
}

// we don't need update anymore
//func (op *Operator) UpdateEngress(ctx context.Context, oldEngress, newEngress *api.Ingress) {
//	op.AddEngress(ctx, newEngress)
//}

func (op *Operator) DeleteEngress(ctx context.Context, engress *api.Ingress) {
	ctrl := ingress.NewController(ctx, op.KubeClient, op.CRDClient, op.VoyagerClient, op.PromClient, op.ServiceLister, op.EndpointsLister, op.Opt, engress)
	ctrl.Delete()
}
