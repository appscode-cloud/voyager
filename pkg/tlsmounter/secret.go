package tlsmounter

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	ioutilz "github.com/appscode/go/ioutil"
	"github.com/appscode/go/log"
	"github.com/appscode/voyager/pkg/eventer"
	"github.com/golang/glog"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rt "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/workqueue"
)

func (c *Controller) initSecretWatcher() {
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (rt.Object, error) {
			return c.k8sClient.CoreV1().Secrets(c.options.IngressRef.Namespace).List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.k8sClient.CoreV1().Secrets(c.options.IngressRef.Namespace).Watch(metav1.ListOptions{})
		},
	}

	// create the workqueue
	c.sQueue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "secret")

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the pod key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the Secret than the version which was responsible for triggering the update.
	c.sIndexer, c.sInformer = cache.NewIndexerInformer(lw, &apiv1.Secret{}, c.options.ResyncPeriod, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if r, ok := obj.(*apiv1.Secret); ok {
				if c.isSecretUsedInIngress(r) {
					key, err := cache.MetaNamespaceKeyFunc(obj)
					if err == nil {
						c.sQueue.Add(key)
					}
				}
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			if r, ok := new.(*apiv1.Secret); ok {
				if c.isSecretUsedInIngress(r) {
					key, err := cache.MetaNamespaceKeyFunc(new)
					if err == nil {
						c.sQueue.Add(key)
					}
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				c.sQueue.Add(key)
			}
		},
	}, cache.Indexers{})
}

func (c *Controller) isSecretUsedInIngress(s *apiv1.Secret) bool {
	if s.Namespace != c.options.IngressRef.Namespace {
		return false
	}
	r, err := c.getIngress()
	if err != nil {
		return false
	}
	for _, tls := range r.Spec.TLS {
		if s.Name == tls.SecretRef.Name && (tls.SecretRef.Kind == "Secret" || tls.SecretRef.Kind == "") {
			return true
		}
	}
	return false
}

func (c *Controller) runSecretWatcher() {
	for c.processNextSecret() {
	}
}

func (c *Controller) processNextSecret() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.sQueue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two secrets with the same key are never processed in
	// parallel.
	defer c.sQueue.Done(key)

	// Invoke the method containing the business logic
	err := c.syncSecret(key.(string))
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.sQueue.Forget(key)
		return true
	}
	log.Errorln("Failed to process Secret %v. Reason: %s", key, err)

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.sQueue.NumRequeues(key) < c.options.MaxNumRequeues {
		glog.Infof("Error syncing secret %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.sQueue.AddRateLimited(key)
		return true
	}

	c.sQueue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	glog.Infof("Dropping secret %q out of the queue: %v", key, err)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the secret to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *Controller) syncSecret(key string) error {
	obj, exists, err := c.sIndexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Secret, so that we will see a delete for one secret
		fmt.Printf("Secret %s does not exist anymore\n", key)
	} else {
		secret := obj.(*apiv1.Secret)
		fmt.Printf("Sync/Add/Update for Secret %s\n", secret.GetName())

		return c.mountSecret(secret)
	}
	return nil
}

func (c *Controller) getSecret(name string) (*apiv1.Secret, error) {
	obj, exists, err := c.sIndexer.GetByKey(c.options.IngressRef.Namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, kerr.NewNotFound(apiv1.Resource("secret"), name)
	}
	return obj.(*apiv1.Secret), nil
}

func (c *Controller) projectSecret(r *apiv1.Secret, projections map[string]ioutilz.FileProjection) error {
	var crt *x509.Certificate
	var key *rsa.PrivateKey
	if data, found := r.Data[apiv1.TLSCertKey]; !found {
		return fmt.Errorf("secret %s@%s is missing tls.crt", r.Name, c.options.IngressRef.Namespace)
	} else {
		crts, err := cert.ParseCertsPEM(data)
		if err != nil {
			return err
		}
		crt = crts[0]
	}
	if data, found := r.Data[apiv1.TLSPrivateKeyKey]; !found {
		return fmt.Errorf("secret %s@%s is missing tls.key", r.Name, c.options.IngressRef.Namespace)
	} else {
		ki, err := cert.ParsePrivateKeyPEM(data)
		if err != nil {
			return err
		}
		if k, ok := ki.(*rsa.PrivateKey); ok {
			key = k
		}
	}

	pemPath := filepath.Join(c.options.MountPath, r.Name+".pem")
	if _, err := os.Stat(pemPath); !os.IsNotExist(err) {
		// path/to/whatever exists
		pemBytes, err := ioutil.ReadFile(pemPath)
		if err != nil {
			return err
		}
		crts, err := cert.ParseCertsPEM(pemBytes)
		if err != nil {
			return err
		}
		if !crts[0].Equal(crt) {
			projections[pemPath] = ioutilz.FileProjection{Mode: 0777, Data: certificateToPEMData(crt, key)}
		}
	} else {
		projections[pemPath] = ioutilz.FileProjection{Mode: 0777, Data: certificateToPEMData(crt, key)}
	}
	return nil
}

func (c *Controller) mountSecret(s *apiv1.Secret) error {
	projections := map[string]ioutilz.FileProjection{}
	err := c.projectSecret(s, projections)
	if err != nil {
		return err
	}
	if len(projections) > 0 {
		r, err := c.getIngress()
		if err != nil {
			return err
		}

		c.lock.Lock()
		defer c.lock.Unlock()
		err = c.writer.Write(projections)
		if err != nil {
			c.recorder.Event(
				r.ObjectReference(),
				apiv1.EventTypeWarning,
				eventer.EventReasonIngressTLSMountFailed,
				err.Error(),
			)
			return err
		} else {
			return runCmd(c.options.CmdFile)
		}
	}
	return nil
}
