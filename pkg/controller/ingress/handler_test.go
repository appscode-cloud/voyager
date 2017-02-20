package ingress

/*import (
	"fmt"
	"testing"

	aci "github.com/appscode/k8s-addons/api"
	"github.com/appscode/k8s-addons/client/clientset/fake"
	"github.com/stretchr/testify/assert"
	kapi "k8s.io/kubernetes/pkg/api"
)

func TestEnsureServiceAnnotation(t *testing.T) {
	fakeClient := fake.NewFakeClient()
	_, err := fakeClient.Core().Services("a").Get("b")
	assert.NotNil(t, err)

	svc, err := fakeClient.Core().Services("test-namespace").Create(&kapi.Service{
		ObjectMeta: kapi.ObjectMeta{
			Name: "test-service",
		},
	})

	ensureServiceAnnotations(fakeClient, &aci.Ingress{
		ObjectMeta: kapi.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: aci.ExtendedIngressSpec{
			Backend: &aci.ExtendedIngressBackend{
				ServiceName: "test-service.test-namespace",
			},
		},
	}, "test-namespace", "test-service")

	svc, err = fakeClient.Core().Services("test-namespace").Get("test-service")
	assert.Nil(t, err)
	assert.NotNil(t, svc.Annotations)
	assert.Contains(t, svc.Annotations, aci.ExtendedIngressKey)
	fmt.Println(svc.Annotations)

	ensureServiceAnnotations(fakeClient, &aci.Ingress{
		ObjectMeta: kapi.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		Spec: aci.ExtendedIngressSpec{
			Backend: &aci.ExtendedIngressBackend{
				ServiceName: "test-service.test-namespace",
			},
		},
	}, "test-namespace", "test-service")

	svc, err = fakeClient.Core().Services("test-namespace").Get("test-service")
	assert.Nil(t, err)
	assert.NotNil(t, svc.Annotations)
	assert.Contains(t, svc.Annotations, aci.ExtendedIngressKey)
	fmt.Println(svc.Annotations)

	ensureServiceAnnotations(fakeClient, &aci.Ingress{
		ObjectMeta: kapi.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
	}, "test-namespace", "test-service")

	svc, err = fakeClient.Core().Services("test-namespace").Get("test-service")
	assert.Nil(t, err)
	assert.NotNil(t, svc.Annotations)
	assert.Contains(t, svc.Annotations, aci.ExtendedIngressKey)
	fmt.Println(svc.Annotations)
}
*/
