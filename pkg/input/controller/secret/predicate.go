// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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

	return gutil.IsShootNamespace(secret.Namespace) &&
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
