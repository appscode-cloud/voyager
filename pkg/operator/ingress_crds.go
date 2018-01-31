package operator

import (
	etx "github.com/appscode/go/context"
	"github.com/appscode/go/log"
	core_util "github.com/appscode/kutil/core/v1"
	"github.com/appscode/kutil/meta"
	api "github.com/appscode/voyager/apis/voyager/v1beta1"
	"github.com/appscode/voyager/client/typed/voyager/v1beta1/util"
	api_listers "github.com/appscode/voyager/listers/voyager/v1beta1"
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
			log.Infof("%s %s/%s has changed. Diff: %s", newEngress.APISchema(), newEngress.Namespace, newEngress.Name, diff)

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
	}, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

	op.engLister = api_listers.NewIngressLister(op.engIndexer)
}

func (op *Operator) runEngressWatcher() {
	for op.processNextEngress() {
	}
}

func (op *Operator) processNextEngress() bool {
	key, quit := op.engQueue.Get()
	if quit {
		return false
	}
	defer op.engQueue.Done(key)

	err := op.reconcileEngress(key.(string))
	if err == nil {
		op.engQueue.Forget(key)
		return true
	}
	log.Errorf("Failed to process engress %v. Reason: %s", key, err)

	if op.engQueue.NumRequeues(key) < op.Opt.MaxNumRequeues {
		glog.Infof("Error syncing engress %v: %v", key, err)
		op.engQueue.AddRateLimited(key)
		return true
	}

	op.engQueue.Forget(key)
	runtime.HandleError(err)
	glog.Infof("Dropping engress %q out of the queue: %v", key, err)
	return true
}

func (op *Operator) reconcileEngress(key string) error {
	obj, exists, err := op.engIndexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	if !exists {
		glog.Warningf("Engress %s does not exist anymore\n", key)
		return nil
	}

	engress := obj.(*api.Ingress).DeepCopy()
	engress.Migrate()
	ctrl := ingress.NewController(etx.Background(), op.KubeClient, op.CRDClient, op.VoyagerClient, op.PromClient, op.svcLister, op.epLister, op.Opt, engress)

	if engress.DeletionTimestamp != nil {
		if core_util.HasFinalizer(engress.ObjectMeta, api.VoyagerFinalizer) {
			glog.Infof("Delete for engress %s\n", key)
			ctrl.Delete()
			util.PatchIngress(op.VoyagerClient, engress, func(obj *api.Ingress) *api.Ingress {
				core_util.RemoveFinalizer(obj.ObjectMeta, api.VoyagerFinalizer)
				return obj
			})
		}
	} else {
		glog.Infof("Sync/Add/Update for engress %s\n", key)
		if !core_util.HasFinalizer(engress.ObjectMeta, api.VoyagerFinalizer) {
			util.PatchIngress(op.VoyagerClient, engress, func(obj *api.Ingress) *api.Ingress {
				core_util.AddFinalizer(obj.ObjectMeta, api.VoyagerFinalizer)
				return obj
			})
		}
		if engress.ShouldHandleIngress(op.Opt.IngressClass) {
			return ctrl.Reconcile()
		} else {
			log.Infof("%s %s/%s does not match ingress class", engress.APISchema(), engress.Namespace, engress.Name)
			ctrl.Delete()
		}
	}
	return nil
}
