package watcher

import (
	"testing"

	aci "github.com/appscode/voyager/api"
	"github.com/appscode/voyager/test/testframework"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func init() {
	testframework.Initialize()
}

func TestEnsureResource(t *testing.T) {
	w := &Watcher{
		KubeClient: fake.NewSimpleClientset(
			&extensions.ThirdPartyResource{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Versions: []extensions.APIVersion{
					{
						Name: "v1",
					},
				},
			},
		),
	}
	w.ensureResource()

	data, err := w.KubeClient.ExtensionsV1beta1().ThirdPartyResources().List(metav1.ListOptions{})
	assert.Nil(t, err)
	if data == nil {
		t.Fatal("Item list should not be nil")
	}
	assert.Equal(t, 3, len(data.Items))

	_, err = w.KubeClient.ExtensionsV1beta1().ThirdPartyResources().Get("ingress."+aci.V1beta1SchemeGroupVersion.Group, metav1.GetOptions{})
	assert.Nil(t, err)

	_, err = w.KubeClient.ExtensionsV1beta1().ThirdPartyResources().Get("certificate."+aci.V1beta1SchemeGroupVersion.Group, metav1.GetOptions{})
	assert.Nil(t, err)
}
