// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.controller.secret.actuator", func() {
	const (
		testNs    = "shoot--my-shoot"
		testToken = "my-token"
	)

	var (
		newTestActuator = func() (*actuator, input_data_registry.InputDataRegistry) {
			idr := input_data_registry.NewInputDataRegistry(1*time.Second, logr.Discard())
			actuator := NewActuator(idr, logr.Discard()).(*actuator)
			return actuator, idr
		}
		newTestSecret = func(name string) (*corev1.Secret, []byte) {
			var dataKey string
			var dataValue []byte

			switch name {
			case secretNameCA:
				dataKey = "ca.crt"
				dataValue = testutil.GetExampleCACert(0)
			case secretNameAccessToken:
				dataKey = "token"
				dataValue = []byte(testToken)
			default:
				Fail("Unknown secret name")
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNs,
					Name:      name,
				},
				Data: map[string][]byte{dataKey: dataValue},
			}

			return secret, dataValue
		}
	)

	Describe("CreateOrUpdate", func() {
		It("should add the CA secret, if it does not exist", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, caCertBytes := newTestSecret(secretNameCA)
			ctx := context.Background()

			// Act
			actuator.CreateOrUpdate(ctx, secret)

			// Assert
			actualCert := idr.GetShootCACertificate(testNs)
			Expect(actualCert).NotTo(BeNil())
			Expect(testutil.IsEqualCert(actualCert, caCertBytes)).To(BeTrue())
		})
		It("should add the auth secret, if it does not exist", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, _ := newTestSecret(secretNameAccessToken)
			ctx := context.Background()

			// Act
			actuator.CreateOrUpdate(ctx, secret)

			// Assert
			actualToken := idr.GetShootAuthSecret(testNs)
			Expect(actualToken).NotTo(BeEmpty())
			Expect(actualToken).To(Equal(testToken))
		})
		It("should return no error, and a zero requeue delay, upon successfully adding a secret", func() {
			// Arrange
			actuator, _ := newTestActuator()
			secret, _ := newTestSecret(secretNameCA)
			ctx := context.Background()

			// Act
			requeue, err := actuator.CreateOrUpdate(ctx, secret)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
		})
		It("should update the CA secret, if it already exists", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, caCertBytes := newTestSecret(secretNameCA)
			ctx := context.Background()
			initialCertBytes := testutil.GetExampleCACert(1)
			idr.SetShootCACertificate(testNs, initialCertBytes)

			// Act
			actuator.CreateOrUpdate(ctx, secret)

			// Assert
			actualCert := idr.GetShootCACertificate(testNs)
			Expect(actualCert).NotTo(BeNil())
			Expect(testutil.IsEqualCert(actualCert, caCertBytes)).To(BeTrue())
			Expect(testutil.IsEqualCert(actualCert, initialCertBytes)).To(BeFalse())
		})
		It("should return no error, and a zero requeue delay, upon successfully adding a secret", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, _ := newTestSecret(secretNameCA)
			ctx := context.Background()
			initialCertBytes := testutil.GetExampleCACert(1)
			idr.SetShootCACertificate(testNs, initialCertBytes)

			// Act
			requeue, err := actuator.CreateOrUpdate(ctx, secret)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
		})
	})
	Describe("Delete", func() {
		It("should delete the respective CA cert, and return no error and zero requeue delay", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, _ := newTestSecret(secretNameCA)
			ctx := context.Background()
			initialCertBytes := testutil.GetExampleCACert(1)
			idr.SetShootCACertificate(testNs, initialCertBytes)
			Expect(idr.GetShootCACertificate(testNs)).NotTo(BeNil())

			// Act
			requeue, err := actuator.Delete(ctx, secret)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
			actualCert := idr.GetShootCACertificate(testNs)
			Expect(actualCert).To(BeNil())
		})
		It("should delete the respective auth secret, and return no error and zero requeue delay", func() {
			// Arrange
			actuator, idr := newTestActuator()
			secret, _ := newTestSecret(secretNameAccessToken)
			ctx := context.Background()
			idr.SetShootAuthSecret(testNs, "my-token")
			Expect(idr.GetShootAuthSecret(testNs)).NotTo(BeEmpty())

			// Act
			requeue, err := actuator.Delete(ctx, secret)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
			actualAuthSecret := idr.GetShootAuthSecret(testNs)
			Expect(actualAuthSecret).To(BeEmpty())
		})
	})
})
