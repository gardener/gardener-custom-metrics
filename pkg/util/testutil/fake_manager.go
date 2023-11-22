//nolint:all
package testutil

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type FakeManager struct {
	runnables []manager.Runnable
	Scheme    *runtime.Scheme
	Client    client.Client
}

func NewFakeManager() *FakeManager {
	return &FakeManager{Client: fake.NewClientBuilder().Build(), Scheme: runtime.NewScheme()}
}

func (f *FakeManager) SetFields(_ interface{}) error {
	return nil
}

func (f *FakeManager) GetConfig() *rest.Config {
	panic("implement me")
}

func (f *FakeManager) GetScheme() *runtime.Scheme {
	return f.Scheme
}

func (f *FakeManager) GetClient() client.Client {
	return f.Client
}

func (f *FakeManager) GetFieldIndexer() client.FieldIndexer {
	panic("implement me")
}

func (f *FakeManager) GetCache() cache.Cache {
	panic("implement me")
}

func (f *FakeManager) GetEventRecorderFor(_ string) record.EventRecorder {
	panic("implement me")
}

func (f *FakeManager) GetRESTMapper() meta.RESTMapper {
	panic("implement me")
}

func (f *FakeManager) GetAPIReader() client.Reader {
	panic("implement me")
}

// GetRunnables returns the subset of FakeManagers' runnables, which are assertable to the specified type
func GetRunnables[T manager.Runnable](f *FakeManager) []T {
	var result []T

	for _, r := range f.runnables {
		if t, ok := r.(T); ok {
			result = append(result, t)
		}
	}

	return result
}

func (f *FakeManager) Add(r manager.Runnable) error {
	f.runnables = append(f.runnables, r)
	return nil
}

func (f *FakeManager) Elected() <-chan struct{} {
	panic("implement me")
}

func (f *FakeManager) AddMetricsExtraHandler(_ string, _ http.Handler) error {
	panic("implement me")
}

func (f *FakeManager) AddHealthzCheck(_ string, _ healthz.Checker) error {
	panic("implement me")
}

func (f *FakeManager) AddReadyzCheck(_ string, _ healthz.Checker) error {
	panic("implement me")
}

func (f *FakeManager) Start(_ context.Context) error {
	panic("implement me")
}

func (f *FakeManager) GetWebhookServer() *webhook.Server {
	panic("implement me")
}

func (f *FakeManager) GetLogger() logr.Logger {
	return logr.Discard()
}

func (f *FakeManager) GetControllerOptions() v1alpha1.ControllerConfigurationSpec {
	panic("implement me")
}
