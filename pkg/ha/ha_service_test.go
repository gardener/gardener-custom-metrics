// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ha

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sync/atomic"
	"time"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("HAService", func() {
	const (
		testNs        = "shoot--my-shoot"
		testIPAddress = "1.2.3.4"
		testPort      = 777
	)
	// Helper functions
	var (
		makeEmptyEndpointsObject = func(namespace string) *corev1.Endpoints {
			return &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      app.Name,
					Namespace: namespace,
				},
			}
		}
		arrange = func() (*HAService, *testutil.FakeManager, context.Context, context.CancelFunc) {
			manager := testutil.NewFakeManager()
			ha := NewHAService(manager, testNs, testIPAddress, testPort, logr.Discard())
			ctx, cancel := context.WithCancel(context.Background())
			return ha, manager, ctx, cancel
		}
		createEndpointsObjectOnServer = func(namespace string, client kclient.Client) {
			endpoints := makeEmptyEndpointsObject(namespace)
			Expect(client.Create(context.Background(), endpoints)).To(Succeed())
		}
		waitGetChangedEndpoints = func(ha *HAService, actualEndpoints *corev1.Endpoints) error {
			// This function returns nil if the endpoints object exists and has changed from its initial, unpopulated state

			err := ha.manager.GetClient().Get(
				context.Background(), kclient.ObjectKey{Namespace: ha.namespace, Name: app.Name}, actualEndpoints)
			if err != nil {
				return err
			}
			if actualEndpoints.Subsets == nil {
				return fmt.Errorf("endpoiints object not populated")
			}
			return nil
		}
		expectEndpointsPopulated = func(actualEndpoints *corev1.Endpoints) {
			Expect(actualEndpoints.Labels).NotTo(BeNil())
			Expect(actualEndpoints.Labels["app"]).To(Equal(app.Name))
			Expect(actualEndpoints.Subsets).To(HaveLen(1))
			Expect(actualEndpoints.Subsets[0].Addresses).To(HaveLen(1))
			Expect(actualEndpoints.Subsets[0].Addresses[0].IP).To(Equal(testIPAddress))
			Expect(actualEndpoints.Subsets[0].Ports).To(HaveLen(1))
			Expect(actualEndpoints.Subsets[0].Ports[0].Port).To(Equal(int32(testPort)))
		}
	)

	Describe("Start", func() {
		It("should create/update the respective service endpoints object ", func() {
			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// So, create an empty object in the fake client first.
			createEndpointsObjectOnServer(ha.namespace, ha.manager.GetClient())
			var err error

			// Act
			go func() {
				err = ha.Start(ctx)
			}()

			// Assert
			actualEndpoints := makeEmptyEndpointsObject(ha.namespace)
			Eventually(func() error { return waitGetChangedEndpoints(ha, actualEndpoints) }).Should(Succeed())
			Expect(err).NotTo(HaveOccurred())
			expectEndpointsPopulated(actualEndpoints)
		})

		It("should immediately abort retrying, if the context gets canceled", func() {
			// Arrange
			ha, manager, ctx, cancel := arrange()
			defer cancel()
			var err error
			var isComplete atomic.Bool

			timeAfterChan := make(chan time.Time)
			ha.testIsolation.TimeAfter = func(_ time.Duration) <-chan time.Time {
				return timeAfterChan
			}

			// Act and assert
			go func() {
				err = ha.Start(ctx)
				isComplete.Store(true)
			}()

			timeAfterChan <- time.Now()
			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// Here we rely on this failure to halt the progress of configuring the endpoints objects.
			// In the real world, the cause of the faults would be different, but that should trigger the same retry
			// mechanic.
			Consistently(isComplete.Load).Should(BeFalse())

			cancel()
			Eventually(isComplete.Load).Should(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("canceled")))
			actual := corev1.Endpoints{}
			err = manager.GetClient().Get(context.Background(), kclient.ObjectKey{Namespace: testNs, Name: app.Name}, &actual)
			Expect(err).To(HaveOccurred())
		})

		It("should wait and retry with exponential backoff, if the service endpoints are missing, and succeed "+
			"once they appear", func() {

			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			var err error
			var isComplete atomic.Bool

			timeAfterChan := make(chan time.Time)
			var timeAfterDuration atomic.Int64
			ha.testIsolation.TimeAfter = func(duration time.Duration) <-chan time.Time {
				timeAfterDuration.Store(int64(duration))
				return timeAfterChan
			}

			// Act and assert
			go func() {
				err = ha.Start(ctx)
				isComplete.Store(true)
			}()

			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// Here we rely on this failure to halt the progress of configuring the endpoints objects.
			// In the real world, the cause of the faults would be different, but that should trigger the same retry
			// mechanic.
			Consistently(isComplete.Load).Should(BeFalse())
			Expect(timeAfterDuration.Load()).To(Equal(int64(1 * time.Second)))

			timeAfterChan <- time.Now()
			Consistently(isComplete.Load).Should(BeFalse())
			Expect(timeAfterDuration.Load()).To(Equal(int64(2 * time.Second)))

			createEndpointsObjectOnServer(ha.namespace, ha.manager.GetClient())
			timeAfterChan <- time.Now()

			actualEndpoints := makeEmptyEndpointsObject(ha.namespace)
			Eventually(func() error { return waitGetChangedEndpoints(ha, actualEndpoints) }).Should(Succeed())
			Expect(err).To(Succeed())
			expectEndpointsPopulated(actualEndpoints)
		})

		It("should use exponential backoff", func() {
			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			timeAfterChan := make(chan time.Time)
			var timeAfterDuration atomic.Int64
			ha.testIsolation.TimeAfter = func(duration time.Duration) <-chan time.Time {
				timeAfterDuration.Store(int64(duration))
				return timeAfterChan
			}

			// Act and assert
			go func() {
				_ = ha.Start(ctx)
			}()

			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// Here we rely on this failure to halt the progress of configuring the endpoints objects.
			// In the real world, the cause of the faults would be different, but that should trigger the same retry
			// mechanic.
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

		It("should delete its service endpoint when context closes", func() {
			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			var err error
			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// So, create an empty object in the fake client first.
			createEndpointsObjectOnServer(ha.namespace, ha.manager.GetClient())

			// Act & assert
			go func() {
				err = ha.Start(ctx)
			}()

			// Wait for HAService to update the Endpoints object
			actualEndpoints := makeEmptyEndpointsObject(ha.namespace)
			Eventually(func() error { return waitGetChangedEndpoints(ha, actualEndpoints) }).Should(Succeed())

			cancel()

			// Wait for HAService to delete update the Endpoints object
			Eventually(func() bool {
				err := ha.manager.GetClient().Get(
					context.Background(), kclient.ObjectKey{Namespace: ha.namespace, Name: app.Name}, actualEndpoints)
				return apierrors.IsNotFound(err)
			}).Should(BeTrue())

			Expect(err.Error()).To(ContainSubstring("canceled"))
		})

		It("upon exit, cleanup should not delete the service endpoint if it points to a different IP address", func() {
			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			var err error
			var isComplete atomic.Bool
			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// So, create an empty object in the fake client first.
			createEndpointsObjectOnServer(ha.namespace, ha.manager.GetClient())

			// Act & assert
			go func() {
				err = ha.Start(ctx)
				isComplete.Store(true)
			}()

			// Wait for HAService to update the Endpoints object
			actualEndpoints := makeEmptyEndpointsObject(ha.namespace)
			Eventually(func() error { return waitGetChangedEndpoints(ha, actualEndpoints) }).Should(Succeed())

			// Modify the Endpoints object so it no longer points to our pod
			actualEndpoints.Subsets[0].Addresses[0].IP = "1.1.1.1"
			Expect(ha.manager.GetClient().Update(ctx, actualEndpoints)).To(Succeed())

			cancel()

			// Make sure the HAService did not delete the Endpoints object
			Eventually(isComplete.Load).Should(BeTrue())
			Expect(err.Error()).To(ContainSubstring("canceled"))
			Expect(
				ha.manager.GetClient().Get(
					context.Background(), kclient.ObjectKey{Namespace: ha.namespace, Name: app.Name}, actualEndpoints)).
				To(Succeed())
		})

		It("upon exit, cleanup should succeed if endpoints object is deleted by an external actor", func() {
			// Arrange
			ha, _, ctx, cancel := arrange()
			defer cancel()
			var err error
			var isComplete atomic.Bool
			// Real K8s API HTTP PUT does create/update and works file if the Endpoints object is missing. The update
			// operation of the client type in the test fake library we use, fails if the object is missing.
			// So, create an empty object in the fake client first.
			createEndpointsObjectOnServer(ha.namespace, ha.manager.GetClient())

			// Act & assert
			go func() {
				err = ha.Start(ctx)
				isComplete.Store(true)
			}()

			// Wait for HAService to update the Endpoints object
			actualEndpoints := makeEmptyEndpointsObject(ha.namespace)
			Eventually(func() error { return waitGetChangedEndpoints(ha, actualEndpoints) }).Should(Succeed())

			// Delete the Endpoints object before triggering cleanup. Error should be "context canceled" and not e.g. "not found"
			Expect(ha.manager.GetClient().Delete(ctx, actualEndpoints)).To(Succeed())
			cancel()
			Eventually(isComplete.Load).Should(BeTrue())
			Expect(err.Error()).To(ContainSubstring("canceled"))
		})
	})
})
