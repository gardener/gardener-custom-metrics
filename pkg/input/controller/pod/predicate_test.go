// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("input.controler.pod.predicate", func() {
	const (
		testNs = "shoot--my-shoot"
	)

	var (
		newTestPod = func() *corev1.Pod {
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNs,
					Labels:    map[string]string{"app": "kubernetes", "role": "apiserver"},
				},
			}
		}
	)

	Describe("Create and Delete", func() {
		It("should return true if the event target is a shoot control plane kube-apiserver pod", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())

			// Act
			allowCreate := predicate.Create(event.CreateEvent{Object: newTestPod()})
			allowDelete := predicate.Delete(event.DeleteEvent{Object: newTestPod()})

			// Assert
			Expect(allowCreate).To(BeTrue())
			Expect(allowDelete).To(BeTrue())
		})
		It("should return false if the event target is not a shoot namespace", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			pod := newTestPod()
			pod.Namespace = "not--shoot"

			// Act
			allowCreate := predicate.Create(event.CreateEvent{Object: pod})
			allowDelete := predicate.Delete(event.DeleteEvent{Object: pod})

			// Assert
			Expect(allowCreate).To(BeFalse())
			Expect(allowDelete).To(BeFalse())
		})
		It("should return false if the event target is not labeled accordingly", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			podNoApp := newTestPod()
			podNoApp.Labels["app"] = "not-kubernetes"
			podNoRole := newTestPod()
			podNoRole.Labels["role"] = "not-apiserver"

			// Act
			allowCreateNoApp := predicate.Create(event.CreateEvent{Object: podNoApp})
			allowDeleteNoApp := predicate.Delete(event.DeleteEvent{Object: podNoApp})
			allowCreateNoRole := predicate.Create(event.CreateEvent{Object: podNoRole})
			allowDeleteNoRole := predicate.Delete(event.DeleteEvent{Object: podNoRole})

			// Assert
			Expect(allowCreateNoApp).To(BeFalse())
			Expect(allowDeleteNoApp).To(BeFalse())
			Expect(allowCreateNoRole).To(BeFalse())
			Expect(allowDeleteNoRole).To(BeFalse())
		})
		It("should return false if the event target is not a pod", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Namespace: testNs,
				Labels:    map[string]string{"app": "kubernetes", "role": "apiserver"},
			}}

			// Act
			allowCreate := predicate.Create(event.CreateEvent{Object: secret})
			allowDelete := predicate.Create(event.CreateEvent{Object: secret})

			// Assert
			Expect(allowCreate).To(BeFalse())
			Expect(allowDelete).To(BeFalse())
		})
	})
	Describe("Update", func() {
		It("should return true if the pod IP changed", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldPod := newTestPod()
			newPod := newTestPod()
			newPod.Status.PodIP = "192.168.22.22"

			// Act
			allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

			// Assert
			Expect(allow).To(BeTrue())
		})
		It("should return true if the pod labeling changed from Kapi to not Kapi", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldPod := newTestPod()
			newPod := newTestPod()
			newPod.Labels["role"] = "no-apiserver"

			// Act
			allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

			// Assert
			Expect(allow).To(BeTrue())
		})
		It("should return true if the pod was labeled as Kapi, but the labels were removed", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldPod := newTestPod()
			newPod := newTestPod()
			newPod.Labels = nil

			// Act
			allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

			// Assert
			Expect(allow).To(BeTrue())
		})
		It("should return true if the pod labeling changed from not Kapi to Kapi", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldPod := newTestPod()
			newPod := newTestPod()
			oldPod.Labels["role"] = "no-apiserver"

			// Act
			allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

			// Assert
			Expect(allow).To(BeTrue())
		})
		It("should return false if the event target is a shoot control plane kube-apiserver pod which "+
			"experienced only changes which do not change the identification of the pod as shoot kube-apiserver pod, "+
			"and do not affect metrics scraping", func() {

			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldPod := newTestPod()
			newPod := newTestPod()
			newPod.ObjectMeta.Annotations = map[string]string{"key": "value"}
			newPod.ObjectMeta.Generation = 777
			newPod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
			newPod.Spec.Volumes = []corev1.Volume{{Name: "my-volume"}}

			// Act
			allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

			// Assert
			Expect(allow).To(BeFalse())
		})
		Context("if the event target is a pod which experienced changes which affect metrics scraping:", func() {
			It("should return false if the namespace is not a shoot namespace", func() {
				// Arrange
				predicate := NewPredicate(logr.Discard())
				oldPod := newTestPod()
				newPod := newTestPod()
				newPod.Status.PodIP = "192.168.22.22"
				oldPod.Namespace = "no-shoot"
				newPod.Namespace = "no-shoot"

				// Act
				allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

				// Assert
				Expect(allow).To(BeFalse())
			})
			It("should return false if the event targets are not labelled accordingly", func() {
				// Arrange
				predicate := NewPredicate(logr.Discard())
				oldPod := newTestPod()
				newPod := newTestPod()
				newPod.Status.PodIP = "192.168.22.22"
				oldPod.Labels["role"] = "no-apiserver"
				newPod.Labels["app"] = "no-kubernetes"

				// Act
				allow := predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod})

				// Assert
				Expect(allow).To(BeFalse())
			})

		})
	})
})
