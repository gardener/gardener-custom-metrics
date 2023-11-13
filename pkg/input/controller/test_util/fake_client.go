//nolint:all
package test_util

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFakeClient() *FakeClient {
	fakeClientset := fake.NewSimpleClientset()
	return &FakeClient{Clientset: fakeClientset}
}

type FakeClient struct {
	Clientset *fake.Clientset
	GetFunc   func(ctx context.Context, key client.ObjectKey, obj client.Object) error
}

func (f *FakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return f.GetFunc(ctx, key, obj)
}

func (f *FakeClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	panic("implement me")
}

func (f *FakeClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	panic("implement me")
}

func (f *FakeClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	panic("implement me")
}

func (f *FakeClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	panic("implement me")
}

func (f *FakeClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	panic("implement me")
}

func (f *FakeClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	panic("implement me")
}

func (f *FakeClient) Status() client.StatusWriter {
	panic("implement me")
}

func (f *FakeClient) Scheme() *runtime.Scheme {
	panic("implement me")
}

func (f *FakeClient) RESTMapper() meta.RESTMapper {
	panic("implement me")
}
