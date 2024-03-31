// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ha

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("HAService", func() {
	const (
		testNs        = "shoot--my-shoot"
		testIPAddress = "1.2.3.4"
		testPort      = 777
	)

	Describe("Start", func() {
		It("should set the respective service endpoints ", func() {
			// Arrange
			manager := testutil.NewFakeManager()
			ha := NewHAService(manager, testNs, testIPAddress, testPort, logr.Discard())

			endpoints := &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app.Name,
					Namespace: ha.namespace,
				},
			}
			Expect(ha.manager.GetClient().Create(context.Background(), endpoints)).To(Succeed())

			// Act
			err := ha.Start(context.Background())

			// Assert
			Expect(err).To(Succeed())
			actual := corev1.Endpoints{}
			manager.GetClient().Get(context.Background(), kclient.ObjectKey{Namespace: testNs, Name: app.Name}, &actual)
			Expect(actual.Labels).NotTo(BeNil())
			Expect(actual.Labels["app"]).To(Equal(app.Name))
			Expect(actual.Subsets).To(HaveLen(1))
			Expect(actual.Subsets[0].Addresses).To(HaveLen(1))
			Expect(actual.Subsets[0].Addresses[0].IP).To(Equal(testIPAddress))
			Expect(actual.Subsets[0].Ports).To(HaveLen(1))
			Expect(actual.Subsets[0].Ports[0].Port).To(Equal(int32(testPort)))
		})

		It("should wait and retry with exponential backoff, if the service endpoints are missing, and succeed "+
			"once they appear", func() {

			// Arrange
			manager := testutil.NewFakeManager()
			ha := NewHAService(manager, testNs, testIPAddress, testPort, logr.Discard())
			timeAfterChan := make(chan time.Time)
			var timeAfterDuration atomic.Int64
			ha.testIsolation.TimeAfter = func(duration time.Duration) <-chan time.Time {
				timeAfterDuration.Store(int64(duration))
				return timeAfterChan
			}
			var err error
			var isComplete atomic.Bool

			// Act and assert
			go func() {
				err = ha.Start(context.Background())
				isComplete.Store(true)
			}()

			Consistently(isComplete.Load).Should(BeFalse())
			Expect(timeAfterDuration.Load()).To(Equal(int64(1 * time.Second)))

			timeAfterChan <- time.Now()
			Consistently(isComplete.Load).Should(BeFalse())
			Expect(timeAfterDuration.Load()).To(Equal(int64(2 * time.Second)))

			endpoints := &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app.Name,
					Namespace: ha.namespace,
				},
			}
			Expect(ha.manager.GetClient().Create(context.Background(), endpoints)).To(Succeed())

			timeAfterChan <- time.Now()

			Eventually(isComplete.Load).Should(BeTrue())
			Expect(err).To(Succeed())
			actual := corev1.Endpoints{}
			manager.GetClient().Get(context.Background(), kclient.ObjectKey{Namespace: testNs, Name: app.Name}, &actual)
			Expect(actual.Subsets).To(HaveLen(1))
			Expect(actual.Subsets[0].Addresses).To(HaveLen(1))
			Expect(actual.Subsets[0].Addresses[0].IP).To(Equal(testIPAddress))
		})

		It("should immediately abort retrying, if the context gets canceled", func() {
			// Arrange
			manager := testutil.NewFakeManager()
			ha := NewHAService(manager, testNs, testIPAddress, testPort, logr.Discard())

			timeAfterChan := make(chan time.Time)
			ha.testIsolation.TimeAfter = func(_ time.Duration) <-chan time.Time {
				return timeAfterChan
			}

			var err error
			var isComplete atomic.Bool
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act and assert
			go func() {
				err = ha.Start(ctx)
				isComplete.Store(true)
			}()

			timeAfterChan <- time.Now()
			Consistently(isComplete.Load).Should(BeFalse())

			cancel()
			Eventually(isComplete.Load).Should(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("canceled")))
			actual := corev1.Endpoints{}
			err = manager.GetClient().Get(context.Background(), kclient.ObjectKey{Namespace: testNs, Name: app.Name}, &actual)
			Expect(err).To(HaveOccurred())
		})

		It("should use exponential backoff", func() {

			// Arrange
			manager := testutil.NewFakeManager()
			ha := NewHAService(manager, testNs, testIPAddress, testPort, logr.Discard())
			timeAfterChan := make(chan time.Time)
			var timeAfterDuration atomic.Int64
			ha.testIsolation.TimeAfter = func(duration time.Duration) <-chan time.Time {
				timeAfterDuration.Store(int64(duration))
				return timeAfterChan
			}

			// Act and assert
			go func() {
				ha.Start(context.Background())
			}()

			expectedPeriod := 1 * time.Second
			expectedMax := 5 * time.Minute
			for i := 0; i < 20; i++ {
				Eventually(timeAfterDuration.Load).Should(Equal(int64(expectedPeriod)))
				expectedPeriod *= 2
				if expectedPeriod > expectedMax {
					expectedPeriod = expectedMax
				}
				timeAfterChan <- time.Now()
			}
			Consistently(timeAfterDuration.Load).Should(Equal(int64(expectedMax)))
		})
	})
})
