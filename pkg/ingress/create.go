package ingress

import (
	"fmt"

	"github.com/appscode/go/log"
	tools "github.com/appscode/kube-mon"
	"github.com/appscode/kube-mon/agents"
	mon_api "github.com/appscode/kube-mon/api"
	"github.com/appscode/kutil"
	core_util "github.com/appscode/kutil/core/v1"
	meta_util "github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/analytics"
	api "github.com/appscode/voyager/apis/voyager/v1beta1"
	"github.com/appscode/voyager/pkg/config"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	TLSCertificateVolumeName = "voyager-certdir"
	ErrorFilesVolumeName     = "voyager-errorfiles"
	ErrorFilesLocation       = "/srv/voyager/errorfiles"
	ErrorFilesCommand        = "errorfile"
)

func (c *controller) ensureConfigMap() (*core.ConfigMap, kutil.VerbType, error) {
	meta := metav1.ObjectMeta{
		Name:      c.Ingress.OffshootName(),
		Namespace: c.Ingress.Namespace,
	}
	return core_util.CreateOrPatchConfigMap(c.KubeClient, meta, func(obj *core.ConfigMap) *core.ConfigMap {
		obj.Annotations = map[string]string{
			api.OriginAPISchema: c.Ingress.APISchema(),
			api.OriginName:      c.Ingress.GetName(),
		}
		obj.Data = map[string]string{
			"haproxy.cfg": c.HAProxyConfig,
		}
		return obj
	})
}

func (c *controller) ensureRBAC() error {
	if err := c.ensureServiceAccount(); err != nil {
		return err
	}
	if err := c.ensureRoles(); err != nil {
		return err
	}
	if err := c.ensureRoleBinding(); err != nil {
		return err
	}
	return nil
}

func (c *controller) getExporterSidecar() (*core.Container, error) {
	if !c.Ingress.Stats() {
		return nil, nil // Don't add sidecar is stats is not exposed.
	}
	monSpec, err := tools.Parse(c.Ingress.Annotations, api.EngressKey, api.DefaultExporterPortNumber)
	if err != nil {
		return nil, err
	}
	if monSpec != nil && monSpec.Prometheus != nil {
		return &core.Container{
			Name: "exporter",
			Args: append([]string{
				"export",
				fmt.Sprintf("--address=:%d", monSpec.Prometheus.Port),
				fmt.Sprintf("--analytics=%v", config.EnableAnalytics),
			}, config.LoggerOptions.ToFlags()...),
			Env: []core.EnvVar{
				{
					Name:  analytics.Key,
					Value: config.AnalyticsClientID,
				},
			},
			Image:           c.Opt.ExporterImage(),
			ImagePullPolicy: core.PullIfNotPresent,
			Ports: []core.ContainerPort{
				{
					Name:          api.ExporterPortName,
					Protocol:      core.ProtocolTCP,
					ContainerPort: int32(monSpec.Prometheus.Port),
				},
			},
		}, nil
	}
	return nil, nil
}

func (c *controller) ensureStatsService() (*core.Service, kutil.VerbType, error) {
	meta := metav1.ObjectMeta{
		Name:      c.Ingress.StatsServiceName(),
		Namespace: c.Ingress.Namespace,
	}

	return core_util.CreateOrPatchService(c.KubeClient, meta, func(in *core.Service) *core.Service {
		in.Labels = c.Ingress.StatsLabels()
		if in.Annotations == nil {
			in.Annotations = map[string]string{}
		}
		in.Annotations[api.OriginAPISchema] = c.Ingress.APISchema()
		in.Annotations[api.OriginName] = c.Ingress.GetName()

		in.Spec.Selector = c.Ingress.OffshootLabels()

		desired := []core.ServicePort{
			{
				Name:       api.StatsPortName,
				Protocol:   core.ProtocolTCP,
				Port:       int32(c.Ingress.StatsPort()),
				TargetPort: intstr.FromString(api.StatsPortName),
			},
		}
		monSpec, err := tools.Parse(c.Ingress.Annotations, api.EngressKey, api.DefaultExporterPortNumber)
		if err == nil && monSpec != nil && monSpec.Prometheus != nil {
			desired = append(desired, core.ServicePort{
				Name:       api.ExporterPortName,
				Protocol:   core.ProtocolTCP,
				Port:       int32(monSpec.Prometheus.Port),
				TargetPort: intstr.FromString(api.ExporterPortName),
			})
		}
		in.Spec.Ports = core_util.MergeServicePorts(in.Spec.Ports, desired)
		return in
	})
}

func (c *controller) ensureOriginAnnotations(annotation map[string]string) (map[string]string, bool) {
	needsUpdate := false

	// Copy the given map to avoid updating the original annotations
	ret := annotation
	if ret == nil {
		ret = make(map[string]string)
	}

	if val := ret[api.OriginAPISchema]; val != c.Ingress.APISchema() {
		needsUpdate = true
		ret[api.OriginAPISchema] = c.Ingress.APISchema()
	}

	if val := ret[api.OriginName]; val != c.Ingress.GetName() {
		needsUpdate = true
		ret[api.OriginName] = c.Ingress.GetName()
	}
	return ret, needsUpdate
}

func (c *controller) ensureMonitoringAgent(monSpec *mon_api.AgentSpec) (kutil.VerbType, error) {
	agent := agents.New(monSpec.Agent, c.KubeClient, c.CRDClient, c.PromClient)

	// if agent-type changed, delete old agent
	// do this before applying new agent-type annotation
	// ignore err here
	if err := c.ensureMonitoringAgentDeleted(agent); err != nil {
		log.Errorf("failed to delete old monitoring agent, reason: %s", err)
	}

	// create/update new agent
	// set agent-type annotation to stat-service
	vt, err := agent.CreateOrUpdate(c.Ingress.StatsAccessor(), monSpec)
	if err == nil {
		err = c.setNewAgentType(agent.GetType())
	}
	return vt, err
}

func (c *controller) getOldAgent() (mon_api.Agent, error) {
	// get stat service
	svc, err := c.KubeClient.CoreV1().Services(c.Ingress.Namespace).Get(c.Ingress.StatsServiceName(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get stat service %s, reason: %s", c.Ingress.StatsServiceName(), err.Error())
	}
	agentType, err := meta_util.GetString(svc.Annotations, mon_api.KeyAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent type, reason: %s", err.Error())
	}
	return agents.New(mon_api.AgentType(agentType), c.KubeClient, c.CRDClient, c.PromClient), nil
}

func (c *controller) setNewAgentType(agentType mon_api.AgentType) error {
	svc, err := c.KubeClient.CoreV1().Services(c.Ingress.Namespace).Get(c.Ingress.StatsServiceName(), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get stat service %s, reason: %s", c.Ingress.StatsServiceName(), err.Error())
	}
	_, _, err = core_util.PatchService(c.KubeClient, svc, func(in *core.Service) *core.Service {
		in.Annotations = core_util.UpsertMap(in.Annotations, map[string]string{
			mon_api.KeyAgent: string(agentType),
		})
		return in
	})
	return err
}
