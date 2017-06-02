package certificates

import (
	"time"

	"github.com/appscode/errors"
	acs "github.com/appscode/voyager/client/clientset"
	"github.com/benbjohnson/clock"
	"k8s.io/kubernetes/pkg/api"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
)

type CertificateSyncer struct {
	CertificateController
	Time clock.Clock
}

func NewCertificateSyncer(c clientset.Interface, a acs.ExtensionInterface) *CertificateSyncer {
	return &CertificateSyncer{
		CertificateController: *NewController(c, a),
		Time: clock.New(),
	}
}

func (c *CertificateSyncer) RunSync() error {
	for {
		select {
		case <-c.Time.After(time.Hour * 24):
			certificates, err := c.ExtClient.Certificate(api.NamespaceAll).List(api.ListOptions{})
			if err != nil {
				return errors.FromErr(err).Err()
			}
			for _, cert := range certificates.Items {
				c.process(&cert)
			}
		}
	}
}
