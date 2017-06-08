package ingress

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/appscode/log"
	aci "github.com/appscode/voyager/api"
	acs "github.com/appscode/voyager/client/clientset"
	"github.com/appscode/voyager/pkg/stash"
	"github.com/appscode/voyager/third_party/forked/cloudprovider"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
)

const (
	VoyagerPrefix = "voyager-"

	stickySession = aci.EngressKey + "/" + "sticky-session"

	// LB stats options
	StatsOn          = aci.EngressKey + "/" + "stats"
	StatsPort        = aci.EngressKey + "/" + "stats-port"
	StatsSecret      = aci.EngressKey + "/" + "stats-secret-name"
	StatsServiceName = aci.EngressKey + "/" + "stats-service-name"
	DefaultStatsPort = 1936

	// Daemon, Persistent, LoadBalancer
	LBType = aci.EngressKey + "/" + "type"

	LBTypeNodePort = "NodePort"
	LBTypeHostPort = "HostPort"
	// Deprecated, use LBTypeHostPort
	LBTypeDaemon       = "Daemon"
	LBTypeLoadBalancer = "LoadBalancer" // default

	// Runs HAProxy on a specific set of a hosts.
	NodeSelector = aci.EngressKey + "/" + "node-selector"

	// Replicas specify # of HAProxy pods run (default 1)
	Replicas = aci.EngressKey + "/" + "replicas"

	// LoadBalancer mode exposes HAProxy via a type=LoadBalancer service. This is the original version implemented by @sadlil
	// Uses nodeport and Cloud LoadBalancer exists beyond single HAProxy run
	LoadBalancerPersist = aci.EngressKey + "/" + "persist" // "" or IP or non-empty

	// BackendWeight is the weight value of a Pod that was
	// addressed by the Endpoint, this weight will be added to server backend.
	// Traffic will be forwarded according to there weight.
	BackendWeight = aci.EngressKey + "/" + "backend-weight"

	// https://github.com/appscode/voyager/issues/103
	// ServiceAnnotations is user provided annotations map that will be
	// applied to the service of that LoadBalancer.
	// ex: "ingress.appscode.com/service.annotation": {"key": "val"}
	ServiceAnnotations = aci.EngressKey + "/" + "annotations-service"

	// PodAnnotations is user provided annotations map that will be
	// applied to the Pods (Deployment/ DaemonSet) of that LoadBalancer.
	// ex: "ingress.appscode.com/service.annotation": {"key": "val"}
	PodAnnotations = aci.EngressKey + "/" + "annotations-pod"

	// Preserves source IP for LoadBalancer type ingresses. The actual configuration
	// generated depends on the underlying cloud provider.
	//
	//  - gce, gke, azure: Adds annotation service.beta.kubernetes.io/external-traffic: OnlyLocal
	// to services used to expose HAProxy.
	// ref: https://kubernetes.io/docs/tutorials/services/source-ip/#source-ip-for-services-with-typeloadbalancer
	//
	// - aws: Enforces the use of the PROXY protocol over any connection accepted by any of
	// the sockets declared on the same line. Versions 1 and 2 of the PROXY protocol
	// are supported and correctly detected. The PROXY protocol dictates the layer
	// 3/4 addresses of the incoming connection to be used everywhere an address is
	// used, with the only exception of "tcp-request connection" rules which will
	// only see the real connection address. Logs will reflect the addresses
	// indicated in the protocol, unless it is violated, in which case the real
	// address will still be used.  This keyword combined with support from external
	// components can be used as an efficient and reliable alternative to the
	// X-Forwarded-For mechanism which is not always reliable and not even always
	// usable. See also "tcp-request connection expect-proxy" for a finer-grained
	// setting of which client is allowed to use the protocol.
	// ref: https://github.com/kubernetes/kubernetes/blob/release-1.5/pkg/cloudprovider/providers/aws/aws.go#L79
	KeepSourceIP = aci.EngressKey + "/" + "keep-source-ip"

	// Annotations applied to resources offshoot from an ingress
	OriginAPISchema = aci.EngressKey + "/" + "origin-api-schema" // APISchema = {APIGroup}/{APIVersion}
	OriginName      = aci.EngressKey + "/" + "origin-name"
)

type annotation map[string]string

func (s annotation) StickySession() bool {
	_, ok := s[stickySession]
	return ok
}

func (s annotation) Stats() bool {
	_, ok := s[StatsOn]
	return ok
}

func (s annotation) StatsSecretName() string {
	v, _ := s[StatsSecret]
	return v
}

func (s annotation) StatsPort() int {
	v, ok := s[StatsPort]
	if !ok {
		return DefaultStatsPort
	}
	if port, err := strconv.Atoi(v); err == nil {
		return port
	}
	return DefaultStatsPort
}

func (s annotation) StatsServiceName(ingName string) string {
	v, ok := s[StatsServiceName]
	if !ok {
		return ingName + "-stats"
	}
	return v
}

func (s annotation) LBType() string {
	if v, ok := s[LBType]; ok {
		return v
	}
	return LBTypeLoadBalancer
}

func (s annotation) Replicas() int32 {
	if v, ok := s[Replicas]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return int32(n)
		}
		return 1
	}
	return 1
}

func (s annotation) NodeSelector() map[string]string {
	if v, ok := s[NodeSelector]; ok {
		return ParseNodeSelector(v)
	}
	v, _ := s[aci.EngressKey+"/"+"daemon.nodeSelector"]
	return ParseNodeSelector(v)
}

func (s annotation) Persist() string {
	if v, ok := s[aci.EngressKey+"/"+"ip"]; ok {
		return v
	}
	v, _ := s[LoadBalancerPersist]
	return v
}

func (s annotation) ServiceAnnotations(provider, lbType string) (map[string]string, bool) {
	m, ok := getTargetAnnotations(s, ServiceAnnotations)
	if ok && lbType == LBTypeLoadBalancer && s.KeepSourceIP() {
		switch provider {
		case "aws":
			// ref: https://github.com/kubernetes/kubernetes/blob/release-1.5/pkg/cloudprovider/providers/aws/aws.go#L79
			m["service.beta.kubernetes.io/aws-load-balancer-proxy-protocol"] = "*"
		case "gce", "gke", "azure":
			// ref: https://kubernetes.io/docs/tutorials/services/source-ip/#source-ip-for-services-with-typeloadbalancer
			m["service.beta.kubernetes.io/external-traffic"] = "OnlyLocal"
		}
	}
	return m, ok
}

func (s annotation) PodsAnnotations() (map[string]string, bool) {
	return getTargetAnnotations(s, PodAnnotations)
}

func (s annotation) KeepSourceIP() bool {
	v, _ := s[KeepSourceIP]
	return strings.ToLower(v) == "true"
}

func getTargetAnnotations(s annotation, key string) (map[string]string, bool) {
	ans := make(map[string]string)
	if v, ok := s[key]; ok {
		v = strings.TrimSpace(v)
		if err := json.Unmarshal([]byte(v), &ans); err != nil {
			log.Errorln("Failed to Unmarshal", key, err)
			return ans, false
		}

		// Filter all annotation keys that starts with ingress.appscode.com
		filteredMap := make(map[string]string)
		for k, v := range ans {
			if !strings.HasPrefix(strings.TrimSpace(k), aci.EngressKey+"/") {
				filteredMap[k] = v
			}
		}
		return filteredMap, true
	}
	return ans, true
}

type EngressController struct {
	// kubernetes client
	KubeClient   clientset.Interface
	ExtClient    acs.ExtensionInterface
	CloudManager cloudprovider.Interface

	ClusterName  string
	ProviderName string
	IngressClass string

	// Engress object that created or updated.
	Resource *aci.Ingress
	// kube options data
	SecretNames []string

	// contains raw configMap data parsed from the cfg file.
	ConfigData string

	// Ports contains all the ports needed to be opened for the ingress.
	// Those ports will be used to open loadbalancer/firewall.
	// So any interference with underlying endpoints will not cause network update.
	Ports []int
	// contains all the https host names.
	HostFilter []string

	// parsed ingress.
	Parsed *HAProxyOptions

	// endpoint cache store. contains all endpoints will be
	// search with respect to services.
	Storage       *stash.Storage
	EndpointStore cache.StoreToEndpointsLister
	sync.Mutex
}

type KubeOptions struct {
	// name of the cluster the daemon running.
	ClusterName string

	ProviderName string
	// kube options data
	SecretNames []string

	// contains raw configMap data parsed from the cfg file.
	ConfigData string

	// Ports contains all the ports needed to be opened for the ingress.
	// Those ports will be used to open loadbalancer/firewall.
	// So any interference with underlying endpoints will not cause network update.
	Ports []int
}

func (lbc *EngressController) SupportsLoadBalancerType() bool {
	return lbc.ProviderName == "aws" ||
		lbc.ProviderName == "gce" ||
		lbc.ProviderName == "gke" ||
		lbc.ProviderName == "azure" ||
		lbc.ProviderName == "minikube"
}

type HAProxyOptions struct {
	Timestamp int64
	// those options are get from annotations. applied globally
	// in all the sections.

	// stick requests to specified servers.
	Sticky  bool
	SSLCert bool

	// open up load balancer stats
	Stats bool
	// Basic auth to lb stats
	StatsUserName string
	StatsPassWord string
	StatsPort     int

	// Add accept-proxy to bind statements
	AcceptProxy bool

	DefaultBackend *Backend
	HttpsService   []*Service
	HttpService    []*Service
	TCPService     []*TCPService
}

type Service struct {
	Name     string
	AclMatch string
	Host     string
	Backends *Backend
}

type TCPService struct {
	Name        string
	Host        string
	Port        string
	SecretName  string
	PEMName     string
	Backends    *Backend
	ALPNOptions string
}

type Backend struct {
	Name         string   `json:"Name,omitempty"`
	BackendRules []string `json:"BackendRules,omitempty"`
	// Deprecated
	RewriteRules []string `json:"RewriteRules,omitempty"`
	// Deprecated
	HeaderRules []string    `json:"HeaderRules,omitempty"`
	Endpoints   []*Endpoint `json:"Endpoints,omitempty"`
}

type Endpoint struct {
	Name   string
	IP     string
	Port   string
	Weight int
}

// Loadbalancer image is an almost constant type.
// this will only be set at the runtime but only for once.
// once this is set the value can not be changed.
var loadbalancerImage string

func SetLoadbalancerImage(i string) {
	var once sync.Once
	once.Do(
		func() {
			loadbalancerImage = i
		},
	)
}

func GetLoadbalancerImage() string {
	return loadbalancerImage
}
