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
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gutil "github.com/gardener/gardener-custom-metrics/pkg/util/gardener"
)

// NewPredicate creates a predicate filter meant to run against a seed cluster. It allows a secret event if that
// secret is the CA certificate or the metrics scraping access token of a shoot kube-apiserver.
func NewPredicate(log logr.Logger) predicate.Predicate {
	return &secretPredicate{
		log: log.WithName("secret-predicate"),
	}
}

// See NewPredicate
type secretPredicate struct {
	log logr.Logger
}

// Is the object a shoot CP secret, containing the shoot's kube-apiserver CA certificate or metrics scraping access token
func (p *secretPredicate) isRelevantSecret(obj client.Object) bool {
	if obj == nil {
		p.log.Error(nil, "Event has no object")
		return false
	}

	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return false
	}

	return gutil.IsShootCPNamespace(secret.Namespace) &&
		(secret.Name == secretNameCA || secret.Name == secretNameAccessToken)
}

// Create returns true if the event target is a shoot control plane kube-apiserver's CA cert or metrics scraping token
func (p *secretPredicate) Create(e event.CreateEvent) bool {
	return p.isRelevantSecret(e.Object)
}

// Update returns true if the event target is a shoot control plane kube-apiserver's CA cert or metrics scraping token
func (p *secretPredicate) Update(e event.UpdateEvent) (result bool) {
	return p.isRelevantSecret(e.ObjectNew)
}

// Delete returns true if the event target is a shoot control plane kube-apiserver's CA cert or metrics scraping token
func (p *secretPredicate) Delete(e event.DeleteEvent) bool {
	return p.isRelevantSecret(e.Object)
}

// Generic rejects the processing of generic events
func (p *secretPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
