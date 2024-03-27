// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	gcmctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller"
	scrape_target_registry "github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// AddToManager adds a new pod controller to the specified manager.
// dataRegistry is a concurrency-safe data repository where the controller finds data it needs, and stores
// the data it produces.
func AddToManager(
	mgr manager.Manager,
	dataRegistry scrape_target_registry.InputDataRegistry,
	controllerOptions controller.Options,
	log logr.Logger) error {

	return gcmctl.NewControllerFactory().AddNewControllerToManager(mgr, gcmctl.AddArgs{
		Actuator:             NewActuator(dataRegistry, log.WithName("pod-controller")),
		ControllerName:       app.Name + "-pod-controller",
		ControllerOptions:    controllerOptions,
		ControlledObjectType: &corev1.Pod{},
		Predicates:           []predicate.Predicate{NewPredicate(log)},
	})
}
