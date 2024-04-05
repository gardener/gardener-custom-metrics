// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	kmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	gcmctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller"
	scrape_target_registry "github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// AddToManager adds a new secret controller to the specified manager.
// dataRegistry is a concurrency-safe data repository where the controller finds data it needs, and stores
// the data it produces.
func AddToManager(
	manager kmgr.Manager,
	dataRegistry scrape_target_registry.InputDataRegistry,
	controllerOptions controller.Options,
	client client.Client,
	log logr.Logger) error {

	return gcmctl.NewControllerFactory().AddNewControllerToManager(manager, gcmctl.AddArgs{
		Actuator:             NewActuator(client, dataRegistry, log.WithName("secret-controller")),
		ControllerName:       app.Name + "-secret-controller",
		ControllerOptions:    controllerOptions,
		ControlledObjectType: &corev1.Secret{},
		Predicates:           []predicate.Predicate{NewPredicate(log)},
	})
}
