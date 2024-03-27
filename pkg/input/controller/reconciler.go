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

	"github.com/gardener/gardener-custom-metrics/pkg/app"
)

// reconciler implements a reconciler which takes care of plumbing and delegates the real work to an Actuator object
type reconciler struct {
	actuator                  Actuator      // The actual work gets delegated to this actuator
	controlledObjectPrototype client.Object // A prototype instance representing the type of objects reconciled by this reconciler
	client                    client.Client // The k8s client to be used by the reconciler
	log                       logr.Logger
}

// NewReconciler creates a new Reconciler which delegates the real work to the specified Actuator.
func NewReconciler(actuator Actuator, controlledObjectPrototype client.Object, client client.Client, log logr.Logger) reconcile.Reconciler {
	log.V(app.VerbosityVerbose).Info("Creating reconciler")
	return &reconciler{
		actuator:                  actuator,
		controlledObjectPrototype: controlledObjectPrototype,
		client:                    client,
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
