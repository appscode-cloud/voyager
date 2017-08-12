package ingress

import (
	"sync"

	"github.com/appscode/voyager/api"
	_ "github.com/appscode/voyager/api/install"
	acs "github.com/appscode/voyager/client/clientset"
	"github.com/appscode/voyager/pkg/config"
	_ "github.com/appscode/voyager/third_party/forked/cloudprovider/providers"
	pcm "github.com/coreos/prometheus-operator/pkg/client/monitoring/v1alpha1"
	clientset "k8s.io/client-go/kubernetes"
	core "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"
)

type Controller interface {
	IsExists() bool
	Create() error
	Update(mode UpdateMode) error
	UpdateTargetAnnotations(old *api.Ingress, new *api.Ingress) error
	Delete() error
}

type controller struct {
	KubeClient      clientset.Interface
	ExtClient       acs.ExtensionInterface
	PromClient      pcm.MonitoringV1alpha1Interface
	ServiceLister   core.ServiceLister
	EndpointsLister core.EndpointsLister

	recorder record.EventRecorder

	Opt config.Options

	// Engress object that created or updated.
	Ingress *api.Ingress

	// contains raw configMap data parsed from the cfg file.
	HAProxyConfig string

	sync.Mutex
}

func NewController(
	kubeClient clientset.Interface,
	extClient acs.ExtensionInterface,
	promClient pcm.MonitoringV1alpha1Interface,
	services core.ServiceLister,
	endpoints core.EndpointsLister,
	opt config.Options,
	ingress *api.Ingress) Controller {
	switch ingress.LBType() {
	case api.LBTypeHostPort:
		return NewHostPortController(kubeClient, extClient, promClient, services, endpoints, opt, ingress)
	case api.LBTypeNodePort:
		return NewNodePortController(kubeClient, extClient, promClient, services, endpoints, opt, ingress)
	case api.LBTypeLoadBalancer:
		return NewLoadBalancerController(kubeClient, extClient, promClient, services, endpoints, opt, ingress)
	}
	return nil
}
