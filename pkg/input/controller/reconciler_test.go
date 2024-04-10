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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("input.controller.reconciler", func() {
	const (
		testNs      = "shoot--my-shoot"
		testPodName = "my-pod"
	)

	var (
		ctx = context.Background()

		newTestReconciler = func() (reconcile.Reconciler, *fakeActuator, client.Client, *corev1.Pod) {
			actuator := &fakeActuator{}
			fakeClient := fake.NewClientBuilder().Build()
			controlledObjectPrototype := &corev1.Pod{}
			reconciler := NewReconciler(actuator, controlledObjectPrototype, fakeClient, logr.Discard())
			return reconciler, actuator, fakeClient, controlledObjectPrototype
		}
	)

	Describe("Reconcile", func() {
		It("should delegate to the actuator's delete function if the object is missing", func() {
			// Arrange
			reconciler, actuator, _, _ := newTestReconciler()

			// Act
			result, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(result.Requeue).To(BeFalse())
			Expect(int(actuator.CallType)).To(Equal(callTypeDelete))
		})

		It("should use a deep copy of the client's prototype object, not the prototype itself", func() {
			// Arrange
			reconciler, _, fakeClient, podPrototype := newTestReconciler()
			podPrototype.Name = testPodName
			tempPodName := "temp-name"
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      tempPodName,
				Namespace: testNs,
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: tempPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(podPrototype.Name).To(Equal(testPodName))
		})

		It("should pass the name and namespace from the request to actuator", func() {
			// Arrange
			reconciler, actuator, fakeClient, _ := newTestReconciler()
			anotherNamespace := "another-namespace"
			anotherPodName := "another-name"
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      anotherPodName,
				Namespace: anotherNamespace,
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())

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
			reconciler, actuator, fakeClient, _ := newTestReconciler()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:       testPodName,
				Namespace:  testNs,
				Finalizers: []string{"foo"},
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())
			Expect(fakeClient.Delete(ctx, pod)).To(Succeed())

			// Act
			_, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(int(actuator.CallType)).To(Equal(callTypeDelete))
		})

		It("should delegate to the actuator's create or update function, if the object does not have a deletion timestamp", func() {
			// Arrange
			reconciler, actuator, fakeClient, _ := newTestReconciler()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      testPodName,
				Namespace: testNs,
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())

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
			reconciler, actuator, fakeClient, _ := newTestReconciler()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      testPodName,
				Namespace: testNs,
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())
			actuator.RequeueAfter = 1 * time.Minute
			actuator.Err = expectedError

			// Act
			result, err := reconciler.Reconcile(
				ctx, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Equal(expectedError))
			Expect(result.RequeueAfter).To(Equal(1 * time.Minute))
		})

		It("should pass the actuator's requeueAfter to the caller, even if error is nil", func() {
			// Arrange
			reconciler, actuator, fakeClient, _ := newTestReconciler()
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      testPodName,
				Namespace: testNs,
			}}
			Expect(fakeClient.Create(ctx, pod)).To(Succeed())
			actuator.RequeueAfter = 2 * time.Minute

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
