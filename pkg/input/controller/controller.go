// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	kctl "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gutil "github.com/gardener/gardener-custom-metrics/pkg/util/gardener"
)

// Actuator acts upon objects being reconciled by a Reconciler.
type Actuator interface {
	// CreateOrUpdate reconciles object creation or update.
	CreateOrUpdate(context.Context, client.Object) (time.Duration, error)
	// Delete reconciles object deletion.
	Delete(context.Context, client.Object) (time.Duration, error)
}

// AddArgs are the arguments required when adding a controller to a manager.
type AddArgs struct {
	Actuator       Actuator
	ControllerName string
	// ControllerOptions are the controller options to use when creating a controller.
	// The Reconciler field is always overridden with a reconciler created from the given actuator.
	ControllerOptions kctl.Options
	// ControlledObjectType is the object type to watch.
	ControlledObjectType client.Object
	// Predicates are the predicates to use when watching objects.
	Predicates []predicate.Predicate
	// WatchBuilder defines additional watches that should be set up.
	WatchBuilder gutil.WatchBuilder
}

// Factory is used to create new Controller instances. It supports redirecting some function calls, for the purpose of test
// isolation
type Factory struct {
	// Points to kctl.New. Enables replacing the function for the purpose of test isolation.
	newController func(name string, mgr manager.Manager, options kctl.Options) (kctl.Controller, error)
}

// NewControllerFactory creates Factory instances
func NewControllerFactory() *Factory {
	return &Factory{newController: kctl.New}
}

// AddNewControllerToManager creates a new controller and adds it to the specified manager, using the specified args.
func (factory *Factory) AddNewControllerToManager(mgr manager.Manager, args AddArgs) error {
	args.ControllerOptions.Reconciler =
		NewReconciler(args.Actuator, args.ControlledObjectType, mgr.GetClient(), log.Log.WithName(args.ControllerName))

	// Create controller
	controller, err := factory.newController(args.ControllerName, mgr, args.ControllerOptions)
	if err != nil {
		return fmt.Errorf("create controller %s: %w", args.ControllerName, err)
	}

	// Add primary watch
	if err := controller.Watch(&source.Kind{Type: args.ControlledObjectType}, &handler.EnqueueRequestForObject{}, args.Predicates...); err != nil {
		return fmt.Errorf("setup primary watch for controller %s: %w", args.ControllerName, err)
	}

	// Add additional watches to the controller besides the primary one.
	if err := args.WatchBuilder.AddToController(controller); err != nil {
		return fmt.Errorf("setup additional watches for controller %s: %w", args.ControllerName, err)
	}

	return nil
}
