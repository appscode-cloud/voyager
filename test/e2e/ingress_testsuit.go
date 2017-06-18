package e2e

import (
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/appscode/errors"
	"github.com/appscode/go/types"
	"github.com/appscode/log"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

type IngressTestSuit struct {
	t TestSuit
}

func NewIngressTestSuit(t TestSuit) *IngressTestSuit {
	return &IngressTestSuit{
		t: t,
	}
}

func (s *IngressTestSuit) Test() error {
	if err := s.setUp(); err != nil {
		return err
	}
	defer s.cleanUp()

	if err := s.runTests(); err != nil {
		return err
	}
	log.Infoln("Ingress Test Passed")
	return nil
}

func (s *IngressTestSuit) setUp() error {
	_, err := s.t.KubeClient.CoreV1().ReplicationControllers(testServerRc.Namespace).Create(testServerRc)
	if err != nil && !kerr.IsAlreadyExists(err) {
		return errors.New().WithCause(err).Err()
	}

	_, err = s.t.KubeClient.CoreV1().Services(testServerSvc.Namespace).Create(testServerSvc)
	if err != nil && !kerr.IsAlreadyExists(err) {
		return errors.New().WithCause(err).Err()
	}

	for it := 0; it < maxRetries; it++ {
		ep, err := s.t.KubeClient.CoreV1().Endpoints(testServerSvc.Namespace).Get(testServerSvc.Name, metav1.GetOptions{})
		if err == nil {
			if len(ep.Subsets) > 0 {
				if len(ep.Subsets[0].Addresses) > 0 {
					break
				}
			}
		}
		log.Infoln("Waiting for endpoint to be ready for testServer")
		time.Sleep(time.Second * 20)
	}

	log.Infoln("Ingress Test Setup Complete")
	return nil
}

func (s *IngressTestSuit) cleanUp() {
	if s.t.Config.Cleanup {
		log.Infoln("Cleaning up Test Resources")
		s.t.KubeClient.CoreV1().Services(testServerSvc.Namespace).Delete(testServerSvc.Name, &metav1.DeleteOptions{})
		rc, err := s.t.KubeClient.CoreV1().ReplicationControllers(testServerRc.Namespace).Get(testServerRc.Name, metav1.GetOptions{})
		if err == nil {
			rc.Spec.Replicas = types.Int32P(0)
			s.t.KubeClient.CoreV1().ReplicationControllers(testServerRc.Namespace).Update(rc)
			time.Sleep(time.Second * 5)
		}
		s.t.KubeClient.CoreV1().ReplicationControllers(testServerRc.Namespace).Delete(testServerRc.Name, &metav1.DeleteOptions{})
	}
}

func (s *IngressTestSuit) runTests() error {
	ingType := reflect.ValueOf(s)
	serializedMethodName := make([]string, 0)
	if len(s.t.Config.RunOnly) > 0 {
		serializedMethodName = append(serializedMethodName, "TestIngress"+s.t.Config.RunOnly)
	} else {
		for it := 0; it < ingType.NumMethod(); it++ {
			method := ingType.Type().Method(it)
			if strings.HasPrefix(method.Name, "TestIngress") {
				if strings.Contains(method.Name, "Ensure") {
					serializedMethodName = append([]string{method.Name}, serializedMethodName...)
				} else {
					serializedMethodName = append(serializedMethodName, method.Name)
				}
			}
		}
	}

	startTime := time.Now()

	errChan := make(chan error)
	var wg sync.WaitGroup
	limit := make(chan bool, s.t.Config.MaxConcurrentTest)
	for _, name := range serializedMethodName {
		shouldCall := ingType.MethodByName(name)
		if shouldCall.IsValid() {
			limit <- true
			wg.Add(1)
			// Run Test in separate goroutine
			go func(fun reflect.Value, n string) {
				defer func() {
					<-limit
					log.Infoln("Test", n, "FINISHED")
					wg.Done()
				}()

				log.Infoln("================== Running Test ====================", n)
				results := fun.Call([]reflect.Value{})
				if len(results) == 1 {
					err, castOk := results[0].Interface().(error)
					if castOk {
						if err != nil {
							log.Infoln("Test", n, "FAILED with Cause", err)
							errChan <- errors.FromErr(err).WithMessagef("Test Name %s", n).Err()
						}
					}
				}
			}(shouldCall, name)
		}
	}

	// ReadLoop
	errs := make([]error, 0)
	go func() {
		for err := range errChan {
			if err != nil {
				errs = append(errs, err)
			}
		}
	}()

	// Wait For All to Be DONE
	wg.Wait()

	log.Infoln("======================================")
	log.Infoln("TOTAL", len(serializedMethodName))
	log.Infoln("PASSED", len(serializedMethodName)-len(errs))
	log.Infoln("FAILED", len(errs))
	log.Infoln("Time Elapsed", time.Since(startTime).Minutes(), "minutes")
	log.Infoln("======================================")
	if len(errs) > 0 {
		for _, err := range errs {
			if err != nil {
				log.Infoln("Log\n", err)
			}
		}
		return errors.New().WithMessage("Test FAILED").WithCause(errs[0]).Err()
	}
	return nil
}
