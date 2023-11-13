package controller

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	kctl "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	kpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.controller.controller", func() {
	var (
		newTestController = func() (*fakeController, *Factory) {
			var c fakeController
			f := NewControllerFactory()
			f.newController = func(name string, mgr manager.Manager, options kctl.Options) (kctl.Controller, error) {
				c.Name = name
				c.Manager = mgr
				c.Options = options
				return &c, nil
			}

			return &c, f
		}
	)

	Describe("AddNewControllerToManager", func() {
		It("should add a new controller to the manager, using the specified args", func() {
			// Arrange
			manager := testutil.NewFakeManager()
			actuator := &fakeActuator{}
			controller, factory := newTestController()
			prototype := &corev1.Pod{}
			predicate := &fakePredicate{}
			rateLimiter := workqueue.NewMaxOfRateLimiter()

			// Act
			factory.AddNewControllerToManager(manager, AddArgs{
				Actuator:             actuator,
				ControllerName:       "my-controller",
				ControlledObjectType: prototype,
				Predicates:           []kpredicate.Predicate{predicate},
				ControllerOptions: kctl.Options{
					RateLimiter: rateLimiter,
				},
			})

			// Assert
			actualReconciler, ok := controller.Options.Reconciler.(*reconciler)
			Expect(ok).To(BeTrue())
			Expect(actualReconciler.controlledObjectPrototype).To(Equal(prototype))
			Expect(actualReconciler.actuator).To(Equal(actuator))
			Expect(controller.Options.RateLimiter).To(Equal(rateLimiter))
			Expect(controller.Manager).To(Equal(manager))
			Expect(controller.Predicates).To(HaveLen(1))
			Expect(controller.Predicates[0]).To(Equal(predicate))
		})
	})
})

//#region fakeController

type fakeController struct {
	Name       string
	Manager    manager.Manager
	Options    kctl.Options
	Predicates []kpredicate.Predicate
}

func (c *fakeController) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	panic("implement me")
}

func (c *fakeController) Watch(_ source.Source, _ handler.EventHandler, predicates ...kpredicate.Predicate) error {
	c.Predicates = append(c.Predicates, predicates...)
	return nil
}

func (c *fakeController) Start(_ context.Context) error {
	panic("implement me")
}

func (c *fakeController) GetLogger() logr.Logger {
	panic("implement me")
}

type fakePredicate struct {
}

func (p *fakePredicate) Create(_ event.CreateEvent) bool {
	panic("implement me")
}
func (p *fakePredicate) Delete(_ event.DeleteEvent) bool {
	panic("implement me")
}
func (p *fakePredicate) Update(_ event.UpdateEvent) bool {
	panic("implement me")
}
func (p *fakePredicate) Generic(_ event.GenericEvent) bool {
	panic("implement me")
}

//#endregion fakeController
