// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	gcmctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// The pod actuator acts upon kube-apiserver pods, maintaining the information necessary to scrape
// the respective shoot kube-apiserver
type actuator struct {
	log logr.Logger
	// А concurrency-safe data repository. Source of various data used by the controller and also where the controller
	// stores the data it produces.
	dataRegistry input_data_registry.InputDataRegistry
}

// NewActuator creates a new pod actuator.
// dataRegistry: a concurrency-safe data repository, source of various data used by the controller, and also where
// the controller stores the data it produces.
func NewActuator(dataRegistry input_data_registry.InputDataRegistry, log logr.Logger) gcmctl.Actuator {

	log.V(app.VerbosityVerbose).Info("Creating actuator")
	return &actuator{
		dataRegistry: dataRegistry,
		log:          log,
	}
}

// CreateOrUpdate tracks shoot kube-apiserver pod creation and update events, and maintains a record of data which
// is relevant to other components.
// Returns:
//   - If an error is returned, the operation is considered to have failed, and reconciliation will be requeued
//     according to default (exponential) schedule.
//   - If error is nil and the Duration is greater than 0, the operation completed successfully and a following
//     reconciliation will be requeued after the specified Duration.
//   - If error is nil, and the Duration is 0, the operation completed successfully and a following delay-based
//     reconciliation is not necessary.
func (a *actuator) CreateOrUpdate(ctx context.Context, obj client.Object) (time.Duration, error) {
	if !isPodLabeledAsShootKapi(obj) {
		// The pod is still there, but the labels which qualify it as a ShootKapi pod were removed
		return a.Delete(ctx, obj)
	}

	pod, ok := toPod(obj, a.log.WithValues("namespace", obj.GetNamespace(), "name", obj.GetName()))
	if !ok {
		return 0, nil // Do not requeue
	}

	metricsUrl := fmt.Sprintf("https://%s/metrics", pod.Status.PodIP)
	labelsCopy := make(map[string]string, len(pod.Labels))
	for k, v := range pod.Labels {
		labelsCopy[k] = v
	}
	a.dataRegistry.SetKapiData(pod.Namespace, pod.Name, pod.UID, labelsCopy, metricsUrl)

	return 0, nil
}

// Delete tracks shoot kube-apiserver pod deletion events, and deletes the data record maintained for the respective pod.
// Returns:
//   - If an error is returned, the operation is considered to have failed, and reconciliation will be requeued
//     according to default (exponential) schedule.
//   - If error is nil and the Duration is greater than 0, the operation completed successfully and a following
//     reconciliation will be requeued after the specified Duration.
//   - If error is nil, and the Duration is 0, the operation completed successfully and a following delay-based
//     reconciliation is not necessary.
func (a *actuator) Delete(_ context.Context, obj client.Object) (requeueAfter time.Duration, err error) {
	log := a.log.WithValues("namespace", obj.GetNamespace(), "name", obj.GetName())
	pod, ok := toPod(obj, log)
	if !ok {
		return 0, nil // Do not requeue
	}

	if !a.dataRegistry.RemoveKapiData(pod.Namespace, pod.Name) {
		log.V(app.VerbosityInfo).Info("Controller was notified about deletion of a pod it was not currently tracking")
	}

	return 0, nil
}

func toPod(obj client.Object, log logr.Logger) (*corev1.Pod, bool) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		log.Error(nil, "pod actuator: reconciled object is not a pod")
	}

	return pod, ok
}
