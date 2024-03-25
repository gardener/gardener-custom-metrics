// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gutil "github.com/gardener/gardener-custom-metrics/pkg/util/gardener"
)

// NewPredicate creates a predicate filter meant to run against a seed cluster. It allows a pod event if that pod is a
// shoot kube-apiserver.
func NewPredicate(log logr.Logger) predicate.Predicate {
	return &podPredicate{
		log: log.WithName("pod-predicate"),
	}
}

// See NewPredicate
type podPredicate struct {
	log logr.Logger
}

func isPodLabeledAsShootKapi(pod client.Object) bool {
	return pod.GetLabels() != nil && pod.GetLabels()["app"] == "kubernetes" && pod.GetLabels()["role"] == "apiserver"
}

func isKapiPod(pod *corev1.Pod) bool {
	return gutil.IsShootNamespace(pod.Namespace) && isPodLabeledAsShootKapi(pod)
}

// Is the object a shoot CP pod, containing one of shoot's kube-apiserver instances
func (p *podPredicate) isKapiPod(obj client.Object) bool {
	if obj == nil {
		p.log.Error(nil, "Event has no object")
		return false
	}

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}

	return isKapiPod(pod)
}

// Create returns true if the event target is a shoot control plane kube-apiserver pod
func (p *podPredicate) Create(e event.CreateEvent) bool {
	return p.isKapiPod(e.Object)
}

// Update returns true if the event target is a shoot control plane kube-apiserver pod which experienced changes
// which 1) affect metrics scraping, or 2) change the identification of the pod as shoot kube-apiserver pod
func (p *podPredicate) Update(e event.UpdateEvent) (result bool) {
	if e.ObjectNew == nil {
		p.log.Error(nil, "Update event has no new object")
		return false
	}
	if !gutil.IsShootNamespace(e.ObjectNew.GetNamespace()) {
		return false
	}

	isOldLabeledKapi := isPodLabeledAsShootKapi(e.ObjectOld)
	isNewLabeledKapi := isPodLabeledAsShootKapi(e.ObjectNew)

	if !isOldLabeledKapi && !isNewLabeledKapi {
		return false // Pod has nothing to do with ShootKapis
	}

	if isOldLabeledKapi != isNewLabeledKapi {
		return true // The pod is entering/exiting controller oversight. That's reason enough to reconcile.
	}

	if e.ObjectOld == nil {
		p.log.Error(nil, "Update event has no old object")
		return true // We can't tell that we don't need to reconcile. So, just reconcile.
	}

	newPod, ok := e.ObjectNew.(*corev1.Pod)
	if !ok {
		p.log.Error(nil, "Update event's new object was not a pod")
		return false // Doesn't matter if the object changed, the reconciler can't handle the unknown type
	}
	oldPod, ok := e.ObjectOld.(*corev1.Pod)
	if !ok {
		p.log.Error(nil, "Update event's old object was not a pod")
		return true
	}

	return oldPod.Status.PodIP != newPod.Status.PodIP || !reflect.DeepEqual(oldPod.Labels, newPod.Labels)
}

// Delete returns true if the event target is a shoot control plane kube-apiserver pod
func (p *podPredicate) Delete(e event.DeleteEvent) bool {
	return p.isKapiPod(e.Object)
}

// Generic rejects the processing of generic events
func (p *podPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
