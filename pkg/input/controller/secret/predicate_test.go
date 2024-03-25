// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("input.controler.secret.predicate", func() {
	const (
		testNs = "shoot--my-shoot"
	)

	var (
		newTestSecret = func(name string) *corev1.Secret {
			return &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNs,
					Name:      name,
				},
			}
		}
	)

	Describe("Predicate operations", func() {
		It("should return true if the event target is a shoot control plane secret, containing the shoot's "+
			"kube-apiserver CA certificate or metrics scraping access token", func() {

			for _, name := range []string{"ca", "shoot-access-gardener-custom-metrics"} {
				// Arrange
				predicate := NewPredicate(logr.Discard())
				oldSecret := newTestSecret(name)
				newSecret := newTestSecret(name)

				// Act
				allowCreate := predicate.Create(event.CreateEvent{Object: newSecret})
				allowUpdate := predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret})
				allowDelete := predicate.Delete(event.DeleteEvent{Object: newSecret})

				// Assert
				Expect(allowCreate).To(BeTrue())
				Expect(allowUpdate).To(BeTrue())
				Expect(allowDelete).To(BeTrue())
			}
		})
		It("should return false if the event target is not in a shoot namespace", func() {
			for _, name := range []string{"ca", "shoot-access-gardener-custom-metrics"} {
				// Arrange
				predicate := NewPredicate(logr.Discard())
				oldSecret := newTestSecret(name)
				newSecret := newTestSecret(name)
				newSecret.Namespace = "another-ns"

				// Act
				allowCreate := predicate.Create(event.CreateEvent{Object: newSecret})
				allowUpdate := predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret})
				allowDelete := predicate.Delete(event.DeleteEvent{Object: newSecret})

				// Assert
				Expect(allowCreate).To(BeFalse())
				Expect(allowUpdate).To(BeFalse())
				Expect(allowDelete).To(BeFalse())
			}
		})
		It("should return true if the event target is not a secret", func() {
			for _, name := range []string{"ca", "shoot-access-gardener-custom-metrics"} {
				// Arrange
				predicate := NewPredicate(logr.Discard())
				oldSecret := newTestSecret(name)
				newSecret := &corev1.Pod{}

				// Act
				allowCreate := predicate.Create(event.CreateEvent{Object: newSecret})
				allowUpdate := predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret})
				allowDelete := predicate.Delete(event.DeleteEvent{Object: newSecret})

				// Assert
				Expect(allowCreate).To(BeFalse())
				Expect(allowUpdate).To(BeFalse())
				Expect(allowDelete).To(BeFalse())
			}
		})
		It("should return true if the event target is neither a CA cert, nor a metrics scraping token", func() {
			// Arrange
			predicate := NewPredicate(logr.Discard())
			oldSecret := newTestSecret("another-secret")
			newSecret := newTestSecret("another-secret")

			// Act
			allowCreate := predicate.Create(event.CreateEvent{Object: newSecret})
			allowUpdate := predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret})
			allowDelete := predicate.Delete(event.DeleteEvent{Object: newSecret})

			// Assert
			Expect(allowCreate).To(BeFalse())
			Expect(allowUpdate).To(BeFalse())
			Expect(allowDelete).To(BeFalse())
		})
	})
})
