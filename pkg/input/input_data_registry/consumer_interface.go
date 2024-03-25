// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input_data_registry

import (
	"time"

	"k8s.io/apimachinery/pkg/types"
)

//#region ShootKapi interface

// ShootKapi contains metrics for a single kube-apiserver pod
type ShootKapi interface {
	ShootNamespace() string       // ShootNamespace and PodName are immutable and together serve as ID
	PodName() string              // ShootNamespace and PodName are immutable and together serve as ID
	PodLabels() map[string]string // The K8s labels on the pod object
	TotalRequestCountNew() int64  // Most recent value for the number of Kapi requests to this pod, since the pod started.
	TotalRequestCountOld() int64  // The previous value of TotalRequestCountNew. Enables rate-of-change calculations.
	MetricsTimeNew() time.Time    // The point in time to which TotalRequestCountNew refers. Zero when the metrics sample is unavailable.
	MetricsTimeOld() time.Time    // The point in time to which TotalRequestCountOld refers. Zero when the metrics sample is unavailable.
	PodUID() types.UID
}

// kapiDataAdapter adapts the KapiData type to the ShootKapi interface
type kapiDataAdapter struct{ x *KapiData }

func (kapi *kapiDataAdapter) PodName() string              { return kapi.x.PodName() }
func (kapi *kapiDataAdapter) ShootNamespace() string       { return kapi.x.ShootNamespace() }
func (kapi *kapiDataAdapter) PodLabels() map[string]string { return kapi.x.PodLabels }
func (kapi *kapiDataAdapter) TotalRequestCountNew() int64  { return kapi.x.TotalRequestCountNew }
func (kapi *kapiDataAdapter) MetricsTimeNew() time.Time    { return kapi.x.MetricsTimeNew }
func (kapi *kapiDataAdapter) TotalRequestCountOld() int64  { return kapi.x.TotalRequestCountOld }
func (kapi *kapiDataAdapter) MetricsTimeOld() time.Time    { return kapi.x.MetricsTimeOld }
func (kapi *kapiDataAdapter) PodUID() types.UID            { return kapi.x.PodUID }

//#endregion ShootKapi interface

//#region InputDataSource interface

// InputDataSource provides kube-apiserver application metrics data. The scope of one instance is multiple shoots
// on the same seed. All operations are concurrency-safe.
type InputDataSource interface {
	// GetShootKapis lists the known Kapi pods for the shoot identified by shootNamespace. Returns nil if the shoot
	// is unknown to InputDataSource at the time of the call.
	GetShootKapis(shootNamespace string) []ShootKapi

	// AddKapiWatcher subscribes an event handler which gets called when there is a change in the ShootKapi objects on
	// record in the InputDataSource.
	// If shouldNotifyOfPreexisting is true, a KapiEventCreate event will be delivered to the watcher for each ShootKapi
	// which is already in the InputDataSource at the time of the call. If false, the watcher will only be notified of
	// future changes.
	//
	// IMPORTANT:
	// If a goroutine exists which could hold a given lock while calling a method on a given InputDataSource instance,
	// then it is illegal for any KapiWatcher registered on that instance to block, even indirectly, on that same lock.
	// The KapiWatcher is still allowed to e.g. create a separate goroutine which blocks in the lock, as long as it
	// doesn't block waiting on the goroutine.
	AddKapiWatcher(watcher *KapiWatcher, shouldNotifyOfPreexisting bool)

	// RemoveKapiWatcher removes the event watcher, registered by a prior AddKapiWatcher call.
	// The watcher pointer must have the same value as the one provided to said AddKapiWatcher() call.
	// Returns false, if the specified watcher has never been added to the InputDataSource, or was already removed.
	RemoveKapiWatcher(watcher *KapiWatcher) bool
}

// dataSourceAdapter adapts the InputDataRegistry type to the InputDataSource interface
type dataSourceAdapter struct{ x *inputDataRegistry }

func (a *dataSourceAdapter) GetShootKapis(shootNamespace string) []ShootKapi {
	a.x.lock.Lock()
	defer a.x.lock.Unlock()

	shoot := a.x.shoots[shootNamespace]
	if shoot == nil {
		return nil
	}

	// Copy
	var result = make([]ShootKapi, len(shoot.KapiData))
	for i := range shoot.KapiData {
		x := *shoot.KapiData[i]
		result[i] = &kapiDataAdapter{&x}
	}

	return result
}

func (a *dataSourceAdapter) AddKapiWatcher(watcher *KapiWatcher, shouldNotifyOfPreexisting bool) {
	a.x.AddKapiWatcher(watcher, shouldNotifyOfPreexisting)
}

func (a *dataSourceAdapter) RemoveKapiWatcher(watcher *KapiWatcher) bool {
	return a.x.RemoveKapiWatcher(watcher)
}

//#endregion InputDataSource interface

//#region Events

// KapiEventType classifies the events on ShootKapi objects, for which a notification can be exchanged.
type KapiEventType int

const (
	KapiEventCreate KapiEventType = iota // KapiEventCreate indicates that a ShootKapi was added.
	KapiEventDelete                      // KapiEventDelete indicates that the ShootKapi is about to be removed.
)

// KapiWatcher is the type of event handlers subscribing to receive ShootKapi events from an InputDataSource.
// The kapi parameter may point to the actual memory backing the InputDataSource. It is illegal to modify the
// object or access it after the event handler has returned.
// See also: KapiEventType.
type KapiWatcher func(kapi ShootKapi, event KapiEventType)

//#endregion Events
