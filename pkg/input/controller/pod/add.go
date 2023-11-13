// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// AddToManagerWithOptions adds a new pod controller to the specified manager.
// dataRegistry is a concurrency-safe data repository where the controller finds data it needs, and stores
// the data it produces.
func AddToManagerWithOptions(
	manager kmgr.Manager,
	dataRegistry scrape_target_registry.InputDataRegistry,
	controllerOptions *controller.Options,
	client client.Client,
	log logr.Logger) error {

	return gcmctl.NewControllerFactory().AddNewControllerToManager(manager, gcmctl.AddArgs{
		Actuator:             NewActuator(client, dataRegistry, log.WithName("pod-controller")),
		ControllerName:       app.Name + "-pod-controller",
		ControllerOptions:    *controllerOptions,
		ControlledObjectType: &corev1.Pod{},
		Predicates:           []predicate.Predicate{NewPredicate(log)},
	})
}

// AddToManager adds a new pod controller to the specified manager, using default option values.
func AddToManager(manager kmgr.Manager, dataRegistry scrape_target_registry.InputDataRegistry, log logr.Logger) error {
	return AddToManagerWithOptions(
		manager,
		dataRegistry,
		&controller.Options{
			RateLimiter: workqueue.NewMaxOfRateLimiter(
				// Sacrifice some of the responsiveness provided by the default 5ms initial retry rate, to reduce waste
				workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 10*time.Minute),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
			),
		},
		nil,
		log)
}
