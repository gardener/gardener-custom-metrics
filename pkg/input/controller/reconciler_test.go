// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/gardener-custom-metrics/pkg/input/controller/test_util"
)

var _ = Describe("input.controller.reconciler", func() {
	const (
		testNs      = "shoot--my-shoot"
		testPodName = "my-pod"
	)

	var (
		newTestReconciler = func() (reconcile.Reconciler, *fakeActuator, *test_util.FakeClient, *corev1.Pod) {
			actuator := &fakeActuator{}
			client := &test_util.FakeClient{}
			client.GetFunc = func(_ context.Context, _ kclient.ObjectKey, _ kclient.Object) error {
				return nil
			}
			controlledObjectPrototype := &corev1.Pod{}
			reconciler := NewReconciler(actuator, controlledObjectPrototype, client, logr.Discard())
			return reconciler, actuator, client, controlledObjectPrototype
		}
	)

	Describe("Reconcile", func() {
		It("should delegate to the actuator's delete function if the object is missing", func() {
			// Arrange
			reconciler, actuator, client, _ := newTestReconciler()
			client.GetFunc = func(_ context.Context, key kclient.ObjectKey, _ kclient.Object) error {
				return errors.NewNotFound(schema.GroupResource{}, key.Name)
			}
			ctx := context.Background()

			// Act
			result, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(result.Requeue).To(BeFalse())
			Expect(int(actuator.CallType)).To(Equal(callTypeDelete))
		})
		It("should use a deep copy of the client's prototype objet, not the prototype itself", func() {
			// Arrange
			reconciler, _, _, podPrototype := newTestReconciler()
			podPrototype.Name = testPodName
			ctx := context.Background()
			tempPodName := "temp-name"

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: tempPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(podPrototype.Name).To(Equal(testPodName))
		})
		It("should pass the name and namespace from the request to actuator", func() {
			// Arrange
			reconciler, actuator, _, _ := newTestReconciler()
			ctx := context.Background()
			anotherNamespace := "another-namespace"
			anotherPodName := "another-name"

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: anotherNamespace, Name: anotherPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(actuator.Obj.GetNamespace()).To(Equal(anotherNamespace))
			Expect(actuator.Obj.GetName()).To(Equal(anotherPodName))
		})
		It("should delegate to the actuator's delete function, if the object has a deletion timestamp", func() {
			// Arrange
			reconciler, actuator, client, _ := newTestReconciler()
			client.GetFunc = func(_ context.Context, _ kclient.ObjectKey, obj kclient.Object) error {
				t := v1.NewTime(time.Now())
				obj.(*corev1.Pod).DeletionTimestamp = &t
				return nil
			}
			ctx := context.Background()

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(int(actuator.CallType)).To(Equal(callTypeDelete))
		})
		It("should delegate to the actuator's delete function, if the object has a deletion timestamp", func() {
			// Arrange
			reconciler, actuator, _, _ := newTestReconciler()
			ctx := context.Background()

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(int(actuator.CallType)).To(Equal(callTypeCreateOrUpdate))
		})
		It("should pass the actuator's requeueAfter and error response, to the caller", func() {
			// Arrange
			expectedError := errors.NewBadRequest("test error")
			reconciler, actuator, _, _ := newTestReconciler()
			actuator.RequeueAfter = 1 * time.Minute
			actuator.Err = expectedError
			ctx := context.Background()

			// Act
			result, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Equal(expectedError))
			Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		})
		It("should pass the actuator's requeueAfter to the caller, even if error is nil", func() {
			// Arrange
			reconciler, actuator, _, _ := newTestReconciler()
			actuator.RequeueAfter = 2 * time.Minute
			ctx := context.Background()

			// Act
			result, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(BeNil())
			Expect(result.RequeueAfter).To(Equal(2 * time.Minute))
		})
	})
})

//#region fakeActuator

type callType int

const (
	callTypeCreateOrUpdate = iota
	callTypeDelete
)

type fakeActuator struct {
	CallType     callType
	Ctx          context.Context
	Obj          kclient.Object
	RequeueAfter time.Duration
	Err          error
}

func (fa *fakeActuator) CreateOrUpdate(ctx context.Context, obj kclient.Object) (time.Duration, error) {
	fa.CallType = callTypeCreateOrUpdate
	fa.Ctx = ctx
	fa.Obj = obj
	return fa.RequeueAfter, fa.Err
}
func (fa *fakeActuator) Delete(ctx context.Context, obj kclient.Object) (time.Duration, error) {
	fa.CallType = callTypeDelete
	fa.Ctx = ctx
	fa.Obj = obj
	return fa.RequeueAfter, fa.Err
}

//#endregion fakeActuator
