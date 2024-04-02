// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	kmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	gcmctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller"
	scrape_target_registry "github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// DefaultAddOptions are the default AddOptions for AddToManager.
var DefaultAddOptions = AddOptions{
	Controller: controller.Options{
		RateLimiter: workqueue.NewMaxOfRateLimiter(
			// Sacrifice some of the responsiveness provided by the default 5ms initial retry rate, to reduce waste
			workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Minute),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	},
}

// AddOptions are options to apply when adding the Pod controller to the manager.
type AddOptions struct {
	// Controller are the controller.Options.
	Controller controller.Options
}

// AddToManagerWithOptions adds a new pod controller to the specified manager.
// dataRegistry is a concurrency-safe data repository where the controller finds data it needs, and stores
// the data it produces.
func AddToManagerWithOptions(
	manager kmgr.Manager,
	dataRegistry scrape_target_registry.InputDataRegistry,
	opts AddOptions,
	client client.Client,
	log logr.Logger) error {

	return gcmctl.NewControllerFactory().AddNewControllerToManager(manager, gcmctl.AddArgs{
		Actuator:             NewActuator(client, dataRegistry, log.WithName("pod-controller")),
		ControllerName:       app.Name + "-pod-controller",
		ControllerOptions:    opts.Controller,
		ControlledObjectType: &corev1.Pod{},
		Predicates:           []predicate.Predicate{NewPredicate(log)},
	})
}

// AddToManager adds a new pod controller to the specified manager, using default option values.
func AddToManager(manager kmgr.Manager, dataRegistry scrape_target_registry.InputDataRegistry, log logr.Logger) error {
	return AddToManagerWithOptions(
		manager,
		dataRegistry,
		DefaultAddOptions,
		nil,
		log)
}
