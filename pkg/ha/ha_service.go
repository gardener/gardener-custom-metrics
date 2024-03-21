// Package ha takes care of concerns related to running the application in high availability mode.
package ha

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	err := ha.manager.GetClient().Get(ctx, client.ObjectKey{Namespace: ha.namespace, Name: app.Name}, &endpoints)
	if err != nil {
		if !errors.IsNotFound(err) {
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
			return fmt.Errorf("starting HA service: %w", ctx.Err())
		case <-ha.testIsolation.TimeAfter(retryPeriod):
		}

		retryPeriod *= 2
		if retryPeriod > maxRetryPeriod {
			retryPeriod = maxRetryPeriod
		}
	}

	return nil
}
