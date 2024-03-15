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

package secret

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	gcmctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

const (
	secretNameCA          = "ca"
	secretNameAccessToken = "shoot-access-gardener-custom-metrics"
)

// The secret actuator acts upon shoot secrets, maintaining the information necessary to scrape
// the respective shoot kube-apiservers
type actuator struct {
	client client.Client
	log    logr.Logger
	// А concurrency-safe data repository. Source of various data used by the controller and also where the controller
	// stores the data it produces.
	dataRegistry input_data_registry.InputDataRegistry
}

// NewActuator creates a new secret actuator.
// dataRegistry: a concurrency-safe data repository, source of various data used by the controller, and also where
// the controller stores the data it produces.
func NewActuator(
	client client.Client, dataRegistry input_data_registry.InputDataRegistry, log logr.Logger) gcmctl.Actuator {

	log.V(app.VerbosityVerbose).Info("Creating actuator")
	return &actuator{
		client:       client,
		dataRegistry: dataRegistry,
		log:          log,
	}
}

// CreateOrUpdate tracks shoot secret creation and update events, and maintains a record of data which
// is relevant to other components.
// Returns:
//   - If an error is returned, the operation is considered to have failed, and reconciliation will be requeued
//     according to default (exponential) schedule.
//   - If error is nil and the Duration is greater than 0, the operation completed successfully and a following
//     reconciliation will be requeued after the specified Duration.
//   - If error is nil, and the Duration is 0, the operation completed successfully and a following delay-based
//     reconciliation is not necessary.
func (a *actuator) CreateOrUpdate(_ context.Context, obj client.Object) (requeueAfter time.Duration, err error) {
	secret, ok := toSecret(obj, a.log.WithValues("namespace", obj.GetNamespace(), "name", obj.GetName()))
	if !ok {
		return 0, nil // Do not requeue
	}

	if secret.Name == secretNameCA {
		return a.setCACertificate(secret, false)
	}
	if secret.Name == secretNameAccessToken {
		return a.setAuthToken(secret, false)
	}

	return 0, nil
}

// Delete tracks shoot secret deletion events, and deletes the data record maintained for the respective shoot.
// Returns:
//   - If an error is returned, the operation is considered to have failed, and reconciliation will be requeued
//     according to default (exponential) schedule.
//   - If error is nil and the Duration is greater than 0, the operation completed successfully and a following
//     reconciliation will be requeued after the specified Duration.
//   - If error is nil, and the Duration is 0, the operation completed successfully and a following delay-based
//     reconciliation is not necessary.
func (a *actuator) Delete(_ context.Context, obj client.Object) (requeueAfter time.Duration, err error) {
	secret, ok := toSecret(obj, a.log.WithValues("namespace", obj.GetNamespace(), "name", obj.GetName()))
	if !ok {
		return 0, nil // Do not requeue
	}

	if secret.Name == secretNameCA {
		return a.setCACertificate(secret, true)
	}
	if secret.Name == secretNameAccessToken {
		return a.setAuthToken(secret, true)
	}

	return 0, nil
}

// InjectClient implements sigs.k8s.io/controller-runtime/pkg/runtime/inject.Client.InjectClient()
func (a *actuator) InjectClient(client client.Client) error {
	a.client = client
	return nil
}

func (a *actuator) setCACertificate(secret *corev1.Secret, isDeleteOperation bool) (time.Duration, error) {
	if isDeleteOperation {
		a.dataRegistry.SetShootCACertificate(secret.Namespace, nil)
		return 0, nil
	}

	if secret.Data == nil {
		return 0, fmt.Errorf("data missing in CA secret %s/%s", secret.Namespace, secret.Name)
	}

	caData := secret.Data["ca.crt"]
	if len(caData) == 0 {
		return 0, fmt.Errorf("CA data missing in CA secret %s/%s", secret.Namespace, secret.Name)
	}

	a.dataRegistry.SetShootCACertificate(secret.Namespace, caData)
	return 0, nil
}

// Returns: (requeueAfter, error)
func (a *actuator) setAuthToken(secret *corev1.Secret, isDeleteOperation bool) (time.Duration, error) {
	if isDeleteOperation {
		a.dataRegistry.SetShootAuthSecret(secret.Namespace, "")
		return 0, nil
	}

	if secret.Data == nil {
		return 0, fmt.Errorf("data missing in auth secret %s/%s", secret.Namespace, secret.Name)
	}

	tokenData := secret.Data["token"]
	if len(tokenData) == 0 {
		return 0, fmt.Errorf("token data missing in auth secret %s/%s", secret.Namespace, secret.Name)
	}

	a.dataRegistry.SetShootAuthSecret(secret.Namespace, string(tokenData))

	return 0, nil
}

// Returns: (requeueAfter, error)
func toSecret(obj client.Object, log logr.Logger) (*corev1.Secret, bool) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		log.Error(nil, "secret actuator: reconciled object is not a secret")
	}

	return secret, ok
}
