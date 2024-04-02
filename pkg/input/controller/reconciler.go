// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
)

// reconciler implements a reconciler which takes care of plumbing and delegates the real work to an Actuator object
type reconciler struct {
	actuator                  Actuator      // The actual work gets delegated to this actuator
	controlledObjectPrototype client.Object // A prototype instance representing the type of objects reconciled by this reconciler
	client                    client.Client // The k8s client to be used by the reconciler
	reader                    client.Reader // The k8s reader to be used by the reconciler
	log                       logr.Logger
}

// NewReconciler creates a new Reconciler which delegates the real work to the specified Actuator.
func NewReconciler(actuator Actuator, controlledObjectPrototype client.Object, log logr.Logger) reconcile.Reconciler {
	log.V(app.VerbosityVerbose).Info("Creating reconciler")
	return &reconciler{
		actuator:                  actuator,
		controlledObjectPrototype: controlledObjectPrototype,
		log:                       log,
	}
}

// Reconcile implements sigs.k8s.io/controller-runtime/pkg/reconcile.Reconciler.Reconcile()
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	obj := r.controlledObjectPrototype.DeepCopyObject().(client.Object)
	obj.SetName(request.Name)
	obj.SetNamespace(request.Namespace)

	isObjectMissing := false
	if err := r.client.Get(ctx, request.NamespacedName, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("error retrieving object from the server: %w", err)
		}
		isObjectMissing = true
	}

	log := r.log.WithValues("name", obj.GetName(), "namespace", obj.GetNamespace())

	var actionName string
	var actionFunction func(context.Context, client.Object) (time.Duration, error)
	if isObjectMissing || obj.GetDeletionTimestamp() != nil {
		actionName = "deletion"
		actionFunction = r.actuator.Delete
	} else {
		actionName = "creation or update"
		actionFunction = r.actuator.CreateOrUpdate
	}

	log.V(app.VerbosityVerbose).Info("Reconciling object " + actionName)
	requeueAfter, err := actionFunction(ctx, obj)
	if err != nil {
		log.V(app.VerbosityInfo).Info(fmt.Sprintf("Reconciling object %s failed: %s", actionName, err))
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, err
}

//#region Injector methods

// InjectFunc implements controller runtime's Injector interface
func (r *reconciler) InjectFunc(f inject.Func) error {
	return f(r.actuator)
}

// InjectClient implements controller runtime's inject.Client interface to enable the ControllerManager injecting a
// client into the reconciler
func (r *reconciler) InjectClient(client client.Client) error {
	r.client = client
	return nil
}

// InjectAPIReader implements controller runtime's inject.APIReader interface to enable the ControllerManager injecting
// an API reader into the reconciler
func (r *reconciler) InjectAPIReader(reader client.Reader) error {
	r.reader = reader
	return nil
}

//#endregion Injector methods
