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

package e2e

import (
	"testing"
	"time"

	"github.com/appscode/voyager/client/clientset/versioned/scheme"
	"github.com/appscode/voyager/pkg/operator"
	"github.com/appscode/voyager/test/framework"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/cli"
	"kmodules.xyz/client-go/tools/clientcmd"
)

const (
	TestTimeout = 2 * time.Hour
)

var (
	root       *framework.Framework
	invocation *framework.Invocation
)

func RunE2ETestSuit(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TestTimeout)
	junitReporter := reporters.NewJUnitReporter("report.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Voyager E2E Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	scheme.AddToScheme(clientsetscheme.Scheme)
	cli.LoggerOptions.Verbosity = "5"

	options.validate()

	clientConfig, err := clientcmd.BuildConfigFromContext(options.KubeConfig, options.KubeContext)
	Expect(err).NotTo(HaveOccurred())

	operatorConfig := operator.NewOperatorConfig(clientConfig)

	err = options.ApplyTo(operatorConfig)
	Expect(err).NotTo(HaveOccurred())

	root = framework.New(operatorConfig, options.TestNamespace, options.Cleanup)

	if options.OperatorOnly { // run operator locally without running tests
		root.Operator.RunInformers(nil)
	}

	By("Ensuring Test Namespace " + options.TestNamespace)
	err = root.EnsureNamespace()
	Expect(err).NotTo(HaveOccurred())

	invocation = root.Invoke()

	if !meta.PossiblyInCluster() && !options.SelfHostedOperator {
		go root.Operator.RunInformers(nil)
	}

	Eventually(invocation.Ingress.Setup).Should(BeNil())
})

var _ = AfterSuite(func() {
	if !options.Cleanup {
		return
	}
	if invocation != nil {
		invocation.Ingress.Teardown()
	}
	if root != nil {
		root.DeleteNamespace()
	}
})
