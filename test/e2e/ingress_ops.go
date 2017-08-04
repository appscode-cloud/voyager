package e2e

import (
	"errors"
	"time"

	"github.com/appscode/voyager/api"
	"github.com/appscode/voyager/test/framework"
	"github.com/appscode/voyager/test/test-server/testserverclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("IngressOperations", func() {
	var (
		f   *framework.Invocation
		ing *api.Ingress
	)

	BeforeEach(func() {
		f = root.Invoke()
		ing = f.Ingress.GetSkeleton()
		f.Ingress.SetSkeletonRule(ing)
	})

	JustBeforeEach(func() {
		By("Creating ingress with name " + ing.GetName())
		err := f.Ingress.Create(ing)
		Expect(err).NotTo(HaveOccurred())

		f.Ingress.EventuallyStarted(ing).Should(BeTrue())

		By("Checking generated resource")
		Expect(f.Ingress.IsTargetCreated(ing)).Should(BeTrue())
	})

	AfterEach(func() {
		if root.Config.Cleanup {
			f.Ingress.Delete(ing)
		}
	})

	var (
		shouldCreateServiceEntry = func() {
			By("Checking StatusIP for provider" + f.Config.CloudProviderName)
			if f.Config.CloudProviderName == "minikube" {
				Skip("Minikube do not support this")
			}
			// Check Status for ingress
			baseIngress, err := f.VoyagerClient.Ingresses(ing.Namespace).Get(ing.Name)
			Expect(err).NotTo(HaveOccurred())

			svc, err := f.Ingress.GetOffShootService(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(baseIngress.Status.LoadBalancer.Ingress)).Should(Equal(len(svc.Status.LoadBalancer.Ingress)))
			Expect(baseIngress.Status.LoadBalancer.Ingress[0]).Should(Equal(svc.Status.LoadBalancer.Ingress[0]))
		}

		shouldResponseHTTP = func() {
			By("Getting HTTP endpoints")
			eps, err := f.Ingress.GetHTTPEndpoints(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(eps)).Should(BeNumerically(">=", 1))

			err = f.Ingress.DoHTTP(framework.MaxRetry, ing, eps, "GET", "/testpath/ok", func(r *testserverclient.Response) bool {
				return Expect(r.Method).Should(Equal("GET")) &&
					Expect(r.Path).Should(Equal("/testpath/ok"))
			})
			Expect(err).NotTo(HaveOccurred())
		}

		shouldDeleteResource = func() {
			By("Deleting Ingress resource")
			err := f.Ingress.Delete(ing)
			Expect(err).NotTo(HaveOccurred())

			Eventually(f.Ingress.Controller(ing).IsExists, "5m", "10s").Should(BeFalse())
		}

		shouldUpdateLoadbalancer = func() {
			By("Updating Ingress resource")
			uing, err := f.Ingress.Get(ing)
			Expect(err).NotTo(HaveOccurred())

			uing.Spec.Rules[0].HTTP.Paths[0].Path = "/newTestPath"
			err = f.Ingress.Update(uing)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting some time for update to be applied")
			time.Sleep(time.Second * 5)

			By("Getting HTTP endpoints")
			eps, err := f.Ingress.GetHTTPEndpoints(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(eps)).Should(BeNumerically(">=", 1))

			By("Calling new HTTP path")
			err = f.Ingress.DoHTTP(framework.MaxRetry, ing, eps, "GET", "/newTestPath/ok", func(r *testserverclient.Response) bool {
				return Expect(r.Method).Should(Equal("GET")) &&
					Expect(r.Path).Should(Equal("/newTestPath/ok"))
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking old path")
			err = f.Ingress.DoHTTP(framework.NoRetry, ing, eps, "GET", "/testpath/ok", func(r *testserverclient.Response) bool {
				return true
			})
			Expect(err).To(HaveOccurred())
		}

		shouldAddTCPRule = func() {
			By("Updating Ingress resource for TCP")
			uing, err := f.Ingress.Get(ing)
			Expect(err).NotTo(HaveOccurred())

			uing.Spec.Rules[0].HTTP = nil
			uing.Spec.Rules[0].TCP = []api.TCPIngressRuleValue{
				{
					Port: intstr.FromString("4545"),
					Backend: api.IngressBackend{
						ServiceName: f.Ingress.TestServerName(),
						ServicePort: intstr.FromString("4545"),
					},
				},
				{
					Port: intstr.FromString("4949"),
					Backend: api.IngressBackend{
						ServiceName: f.Ingress.TestServerName(),
						ServicePort: intstr.FromString("4545"),
					},
				},
			}
			err = f.Ingress.Update(uing)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting some time for update to be applied")
			time.Sleep(time.Second * 5)
			Eventually(func() error {
				svc, err := f.KubeClient.CoreV1().Services(ing.GetNamespace()).Get(ing.OffshootName(), metav1.GetOptions{})
				if err != nil {
					return err
				}
				for _, port := range svc.Spec.Ports {
					if port.Port == 4545 {
						return nil
					}
				}
				return errors.New("TCP port not found")
			}, "5m", "10s").Should(BeNil())

			By("Getting endpoints")
			eps, err := f.Ingress.GetHTTPEndpoints(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(eps)).Should(BeNumerically(">=", 1))

			By("Calling new TCP")
			err = f.Ingress.DoTCP(framework.MaxRetry, ing, eps, func(r *testserverclient.Response) bool {
				return Expect(r.ServerPort).Should(Equal(":4545"))
			})
			Expect(err).NotTo(HaveOccurred())

			err = f.Ingress.CheckTestServersPortAssignments(ing)
			Expect(err).Should(BeNil())
		}

		shouldPersistIP = func() {
			if len(f.Config.LBPersistIP) == 0 {
				Skip("Persistent IP is not provided")
			}
			if !f.Ingress.SupportsServiceIP() {
				Skip("Persistent IP is not supported")
			}

			By("Checking HTTP Response", shouldResponseHTTP)
			oldsvc, err := f.Ingress.GetOffShootService(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(oldsvc.Status.LoadBalancer.Ingress)).Should(Equal(1))

			Expect(f.Ingress.Delete(ing)).NotTo(HaveOccurred())
			By("Wait for resource to be deleted", shouldDeleteResource)
			ing.Annotations[api.LoadBalancerIP] = f.Config.LBPersistIP
			Expect(f.Ingress.Create(ing)).NotTo(HaveOccurred())

			By("Checking HTTP Response", shouldResponseHTTP)
			newsvc, err := f.Ingress.GetOffShootService(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(newsvc.Status.LoadBalancer.Ingress)).Should(Equal(1))

			Expect(newsvc.Status.LoadBalancer.Ingress).Should(Equal(oldsvc.Status.LoadBalancer.Ingress))
		}

		rulesShouldApply = func() {
			By("Getting HTTP endpoints")
			eps, err := f.Ingress.GetHTTPEndpoints(ing)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(eps)).Should(BeNumerically(">=", 1))

			err = f.Ingress.DoHTTP(framework.MaxRetry, ing, eps, "GET", "/testpath/ok", func(r *testserverclient.Response) bool {
				return Expect(r.Method).Should(Equal("GET")) &&
					Expect(r.Path).Should(Equal("/override/testpath/ok")) &&
					Expect(r.RequestHeaders.Get("X-Ingress-Test-Header")).Should(Equal("ingress.appscode.com"))
			})
			Expect(err).NotTo(HaveOccurred())

			err = f.Ingress.DoHTTPWithHeader(framework.MaxRetry,
				ing,
				eps,
				"GET",
				"/testpath/ok",
				map[string]string{
					"X-Ingress-Test-Header": api.GroupName + "/v1beta1",
				},
				func(r *testserverclient.Response) bool {
					return Expect(r.Method).Should(Equal("GET")) &&
						Expect(r.Path).Should(Equal("/override/testpath/ok")) &&
						Expect(r.RequestHeaders.Get("X-Ingress-Test-Header")).Should(Equal(api.GroupName+"/v1beta1"))
				},
			)
			Expect(err).NotTo(HaveOccurred())
		}
	)

	Describe("Create", func() {
		It("Should create Loadbalancer entry", shouldCreateServiceEntry)
		It("Should response HTTP", shouldResponseHTTP)

		Describe("With persistent IP", func() {
			BeforeEach(func() {
				ing.Annotations[api.LoadBalancerIP] = f.Config.LBPersistIP
			})
			It("Should persist service IP", shouldPersistIP)
		})

		Describe("With BackendRules", func() {
			BeforeEach(func() {
				ing.Spec.Rules = []api.IngressRule{
					{
						IngressRuleValue: api.IngressRuleValue{
							HTTP: &api.HTTPIngressRuleValue{
								Paths: []api.HTTPIngressPath{
									{
										Backend: api.IngressBackend{
											ServiceName: f.Ingress.TestServerName(),
											ServicePort: intstr.FromInt(80),
											HeaderRule: []string{
												"X-Ingress-Test-Header ingress.appscode.com",
											},
											RewriteRule: []string{
												`^([^\ :]*)\ /(.*)$ \1\ /override/\2`,
											},
										},
									},
								},
							},
						},
					},
				}
			})

			It("Rules Should Apply", rulesShouldApply)
		})
	})

	Describe("Delete", func() {
		It("Should delete Ingress resource", shouldDeleteResource)
	})

	Describe("Update", func() {
		It("Should update Loadbalancer", shouldUpdateLoadbalancer)
		It("Should add TCP rule", shouldAddTCPRule)
	})
})
