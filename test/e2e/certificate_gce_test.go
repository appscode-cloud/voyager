/*
Copyright The Voyager Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_test

import (
	"context"
	"fmt"
	"os"

	api "voyagermesh.dev/voyager/apis/voyager/v1beta1"
	"voyagermesh.dev/voyager/pkg/certificate"
	"voyagermesh.dev/voyager/test/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CertificateWithDNSProvider", func() {
	var (
		f                *framework.Invocation
		cert             *api.Certificate
		userSecret       *core.Secret
		credentialSecret *core.Secret
	)

	BeforeEach(func() {
		f = root.Invoke()

		skipTestIfSecretNotProvided()
		if !options.TestCertificate {
			Skip("Certificate Test is not enabled")
		}

		userSecret = &core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "user-" + f.Certificate.UniqueName(),
				Namespace: f.Namespace(),
			},
			Data: map[string][]byte{
				api.ACMEUserEmail: []byte("sadlil@appscode.com"),
				api.ACMEServerURL: []byte(certificate.LetsEncryptStagingURL),
			},
		}

		_, err := f.KubeClient.CoreV1().Secrets(userSecret.Namespace).Create(context.TODO(), userSecret, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		f = root.Invoke()

		fmt.Println("TEST_GCE_PROJECT", os.Getenv("TEST_GCE_PROJECT"))
		fmt.Println("TEST_DNS_DOMAINS", os.Getenv("TEST_DNS_DOMAINS"))

		credentialSecret = &core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cred-" + f.Certificate.UniqueName(),
				Namespace: f.Namespace(),
			},
			Data: map[string][]byte{
				"GCE_PROJECT":              []byte(os.Getenv("TEST_GCE_PROJECT")),
				"GCE_SERVICE_ACCOUNT_DATA": []byte(os.Getenv("TEST_GCE_SERVICE_ACCOUNT_DATA")),
			},
		}

		_, err := f.KubeClient.CoreV1().Secrets(credentialSecret.Namespace).Create(context.TODO(), credentialSecret, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	BeforeEach(func() {
		cert = f.Certificate.GetSkeleton()
		cert.Spec = api.CertificateSpec{
			Domains: []string{os.Getenv("TEST_DNS_DOMAINS")},
			ChallengeProvider: api.ChallengeProvider{
				DNS: &api.DNSChallengeProvider{
					Provider:             "googlecloud",
					CredentialSecretName: credentialSecret.Name,
				},
			},
			ACMEUserSecretName: userSecret.Name,
			Storage: api.CertificateStorage{
				Secret: &core.LocalObjectReference{},
			},
		}
	})

	JustBeforeEach(func() {
		By("Creating certificate with" + cert.Name)
		err := f.Certificate.Create(cert)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if options.Cleanup {
			Expect(f.KubeClient.CoreV1().Secrets(userSecret.Namespace).Delete(context.TODO(), userSecret.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())
			Expect(f.KubeClient.CoreV1().Secrets(credentialSecret.Namespace).Delete(context.TODO(), credentialSecret.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())
		}
	})

	Describe("Create", func() {
		It("Should check secret", func() {
			Eventually(func() bool {
				secret, err := f.KubeClient.CoreV1().Secrets(cert.Namespace).Get(context.TODO(), cert.SecretName(), metav1.GetOptions{})
				if err != nil {
					return false
				}
				if _, ok := secret.Data["tls.crt"]; !ok {
					return false
				}
				return true
			}, "20m", "10s").Should(BeTrue())
		})
	})
})

func skipTestIfSecretNotProvided() {
	if len(os.Getenv("TEST_GCE_PROJECT")) == 0 ||
		len(os.Getenv("TEST_GCE_SERVICE_ACCOUNT_DATA")) == 0 {
		Skip("Skipping Test, Secret Not Provided")
	}
}
