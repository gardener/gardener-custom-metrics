// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package ha takes care of concerns related to running the application in high availability mode.
package ha

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/util/errutil"
)

// HAService is the main type of the package. It takes care of concerns related to running the application in high
// availability mode. When running in active/passive replication mode, HAService ensures that all requests go to the
// active replica.
// HAService implements [ctlmgr.Runnable].
// For information about individual fields, see NewHAService().
type HAService struct {
	log              logr.Logger
	manager          ctlmgr.Manager
	namespace        string
	servingIPAddress string
	servingPort      int

	testIsolation testIsolation
}

// Enables redirecting some function calls for the purposes of test isolation
type testIsolation struct {
	// Points to time.After
	TimeAfter func(time.Duration) <-chan time.Time
}

// NewHAService creates a new HAService instance.
//
// manager is the [ctlmgr.Manager] instance which orchestrates the leader election process upon which HA operation relies.
//
// namespace is the K8s namespace in which this process and associated artefacts belong.
//
// servingIPAddress is the IP address at which custom metrics from this process can be consumed.
//
// servingPort is the network port at which custom metrics from this process can be consumed.
func NewHAService(
	manager ctlmgr.Manager, namespace string, servingIPAddress string, servingPort int, parentLogger logr.Logger) *HAService {

	return &HAService{
		log:              parentLogger.WithName("ha"),
		manager:          manager,
		namespace:        namespace,
		servingIPAddress: servingIPAddress,
		servingPort:      servingPort,
		testIsolation:    testIsolation{TimeAfter: time.After},
	}
}

func (ha *HAService) setEndpoints(ctx context.Context) error {
	endpoints := corev1.Endpoints{}
	// Bypass client cache to avoid triggering a cluster wide list-watch for Endpoints - our RBAC does not allow it
	err := ha.manager.GetAPIReader().Get(ctx, client.ObjectKey{Namespace: ha.namespace, Name: app.Name}, &endpoints)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("updating the service endpoint to point to the new leader: retrieving endpoints: %w", err)
		}

		endpoints.ObjectMeta.Namespace = ha.namespace
		endpoints.ObjectMeta.Name = app.Name
	}

	endpoints.ObjectMeta.Labels = map[string]string{"app": app.Name}
	endpoints.Subsets = []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: ha.servingIPAddress}},
		Ports:     []corev1.EndpointPort{{Port: int32(ha.servingPort), Protocol: "TCP"}},
	}}

	err = ha.manager.GetClient().Update(ctx, &endpoints)
	return errutil.Wrap("updating the service endpoint to point to the new leader", err)
}

// Start implements [ctlmgr.Runnable.Start]. The HAService.manager runs this function when this process becomes the
// leader. The function ensures that the single endpoint for the gardener-metrics-provider service points to this
// process' server endpoint, thus ensuring that all requests go to the leader.
func (ha *HAService) Start(ctx context.Context) error {
	retryPeriod := 1 * time.Second
	maxRetryPeriod := 5 * time.Minute

	for err := ha.setEndpoints(ctx); err != nil; err = ha.setEndpoints(ctx) {
		ha.log.V(app.VerbosityError).Error(err, "Failed to set service endpoints")

		select {
		case <-ctx.Done():
			_ = ha.cleanUpServiceEndpoints()
			return fmt.Errorf("starting HA service: %w", ctx.Err())
		case <-ha.testIsolation.TimeAfter(retryPeriod):
		}

		retryPeriod *= 2
		if retryPeriod > maxRetryPeriod {
			retryPeriod = maxRetryPeriod
		}
	}

	<-ctx.Done()
	err := ha.cleanUpServiceEndpoints()
	if err == nil {
		err = ctx.Err()
	}
	return err
}

// cleanUpServiceEndpoints is executed upon ending leadership. Its purpose is to remove the Endpoints object created upon acquiring
// leadership.
func (ha *HAService) cleanUpServiceEndpoints() error {
	// Use our own context. This function executes when the main application context is closed.
	// Also, try to finish before a potential 15 seconds termination grace timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
	defer cancel()
	seedClient := ha.manager.GetClient()

	attempt := 0
	var err error
	for {
		endpoints := corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      app.Name,
				Namespace: ha.namespace,
			},
		}
		err = seedClient.Get(ctx, client.ObjectKeyFromObject(&endpoints), &endpoints)
		if err != nil {
			if apierrors.IsNotFound(err) {
				ha.log.V(app.VerbosityVerbose).Info("The endpoints object cleanup succeeded: the object was missing")
				return nil
			}

			ha.log.V(app.VerbosityInfo).Info("Failed to retrieve the endpoints object", "error", err.Error())
		} else {
			// Avoid data race. We don't want to delete the endpoint if it is sending traffic to a replica other than this one.
			if !ha.isEndpointStillPointingToOurReplica(&endpoints) {
				// Someone else is using the endpoint. We can't perform safe cleanup. Abandon the object.
				ha.log.V(app.VerbosityWarning).Info(
					"Abandoning endpoints object because it was modified by an external actor")
				return nil
			}

			// Only delete the endpoint if it is the resource version for which we confirmed that it points to us.
			deletionPrecondition := client.Preconditions{UID: &endpoints.UID, ResourceVersion: &endpoints.ResourceVersion}
			err = seedClient.Delete(ctx, &endpoints, deletionPrecondition)
			if client.IgnoreNotFound(err) == nil {
				// The endpoint was deleted (even if not by us). We call that successful cleanup.
				ha.log.V(app.VerbosityVerbose).Info("The endpoints object cleanup succeeded")
				return nil
			}
			ha.log.V(app.VerbosityInfo).Info("Failed to delete the endpoints object", "error", err.Error())
		}

		// Deletion request failed, possibly because of a midair collision. Wait a bit and retry.
		attempt++
		if attempt >= 10 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	ha.log.V(app.VerbosityError).Error(err, "All retries to delete the endpoints object failed. Abandoning object.")
	return fmt.Errorf("HAService cleanup: deleting endponts object: retrying failed, last error: %w", err)
}

// Does the endpoints object hold the same values as the ones we previously set to it?
func (ha *HAService) isEndpointStillPointingToOurReplica(endpoints *corev1.Endpoints) bool {
	return len(endpoints.Subsets) == 1 &&
		len(endpoints.Subsets[0].Addresses) == 1 &&
		endpoints.Subsets[0].Addresses[0].IP == ha.servingIPAddress &&
		len(endpoints.Subsets[0].Ports) == 1 &&
		endpoints.Subsets[0].Ports[0].Port == int32(ha.servingPort) &&
		endpoints.Subsets[0].Ports[0].Protocol == corev1.ProtocolTCP
}
