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
package ingress

import (
	"github.com/pkg/errors"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *controller) deleteConfigMap() error {
	c.logger.Infof("Deleting ConfigMap %s/%s", c.Ingress.Namespace, c.Ingress.OffshootName())
	err := c.KubeClient.CoreV1().ConfigMaps(c.Ingress.Namespace).Delete(c.Ingress.OffshootName(), &metav1.DeleteOptions{})
	if err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func (c *controller) ensureServiceDeleted() error {
	c.logger.Infof("Deleting Service %s/%s", c.Ingress.Namespace, c.Ingress.OffshootName())
	err := c.KubeClient.CoreV1().Services(c.Ingress.Namespace).Delete(c.Ingress.OffshootName(), &metav1.DeleteOptions{})
	if err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func (c *controller) ensureStatsServiceDeleted() error {
	c.logger.Infof("Deleting Stats Service %s/%s", c.Ingress.Namespace, c.Ingress.StatsServiceName())
	err := c.KubeClient.CoreV1().Services(c.Ingress.Namespace).Delete(
		c.Ingress.StatsServiceName(),
		&metav1.DeleteOptions{},
	)
	if err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}

func (c *controller) ensureRBACDeleted() error {
	if err := c.ensureRoleBindingDeleted(); err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}

	if err := c.ensureRolesDeleted(); err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}

	if err := c.ensureServiceAccountDeleted(); err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}
	return nil
}
