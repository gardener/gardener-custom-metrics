// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package input_data_registry holds application metrics from shoot kube-apiserver (Kapi) pods and
// information necessary to scrape such metrics.
package input_data_registry

import (
	"crypto/x509"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/types"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
)

//#region Registry element types

// KapiData holds all registry information for a single kube-apiserver pod
type KapiData struct {
	shootNamespace        string            // ShootNamespace and PodName are immutable and together serve as ID
	podName               string            // ShootNamespace and PodName are immutable and together serve as ID
	PodLabels             map[string]string // The K8s labels on the pod object
	MetricsUrl            string            // The URL where metrics for the pod can be scraped
	TotalRequestCountNew  int64             // Most recent value for the number of Kapi requests to this pod, since the pod started.
	MetricsTimeNew        time.Time         // The point in time to which TotalRequestCountNew refers. Zero when the metrics sample is unavailable.
	TotalRequestCountOld  int64             // The previous value of TotalRequestCountNew. Enables rate-of-change calculations.
	MetricsTimeOld        time.Time         // The point in time to which TotalRequestCountOld refers. Zero when the metrics sample is unavailable.
	PodUID                types.UID
	LastMetricsScrapeTime time.Time // The start time of the most recent metrics scrape for the Kapi.
	FaultCount            int       // Number of consecutive failed attempt to obtain metrics for this pod. Reset to zero upon success.
}

// ShootNamespace and PodName jointly identify the KapiData
func (kapi *KapiData) ShootNamespace() string {
	return kapi.shootNamespace
}

// PodName and ShootNamespace jointly identify the KapiData
func (kapi *KapiData) PodName() string {
	return kapi.podName
}

// Copy returns a deep copy.
func (kapi *KapiData) Copy() *KapiData {
	if kapi == nil {
		return nil
	}

	result := &KapiData{
		shootNamespace:        kapi.shootNamespace,
		podName:               kapi.podName,
		PodLabels:             make(map[string]string, len(kapi.PodLabels)),
		MetricsUrl:            kapi.MetricsUrl,
		TotalRequestCountNew:  kapi.TotalRequestCountNew,
		MetricsTimeNew:        kapi.MetricsTimeNew,
		TotalRequestCountOld:  kapi.TotalRequestCountOld,
		MetricsTimeOld:        kapi.MetricsTimeOld,
		PodUID:                kapi.PodUID,
		LastMetricsScrapeTime: kapi.LastMetricsScrapeTime,
		FaultCount:            kapi.FaultCount,
	}

	for k, v := range kapi.PodLabels {
		result.PodLabels[k] = v
	}

	return result
}

// shootData holds all registry information for a single shoot
type shootData struct {
	shootNamespace string // Serves as ID. Immutable.
	AuthSecret     string // Authentication secret for the shoot Kapi. A missing authSecret is represented by an empty string.

	// CertPool containing the shoot Kapi CA certificate. Nil if there is no CA certificate on record for the shoot.
	CACertPool *x509.CertPool

	KapiData []*KapiData // Information about individual Kapi pods
}

// ShootNamespace serves as identifier for the shoot. Immutable.
func (shoot *shootData) ShootNamespace() string {
	return shoot.shootNamespace
}

//#endregion Registry element types

// InputDataRegistry abstracts the inputDataRegistry type, so it can be replaced for testing isolation purposes.
type InputDataRegistry interface {
	// DataSource returns an InputDataSource interface to the registry, which is focused on metrics consumption, and
	// abstracts other details away.
	DataSource() InputDataSource
	// GetKapiData returns a KapiData object which contains the registry's information, specific to the Kapi pod identified
	// by shootNamespace and podName.
	// The output is a deep copy, and fully detached from the registry. If the registry has no information about the
	// specified pod, nil is returned.
	GetKapiData(shootNamespace string, podName string) *KapiData
	// SetKapiData stores registry data specific to the k8s Kapi pod object identified by shootNamespace and podName.
	SetKapiData(
		shootNamespace string, podName string, podUID types.UID, podLabels map[string]string, metricsUrl string)
	// RemoveKapiData deletes all registry data specific to the Kapi pod identified by shootNamespace and podName.
	// The output value is false if the registry did not contain data for the identified pod.
	RemoveKapiData(shootNamespace string, podName string) bool
	// SetKapiMetrics records the current metrics value for the Kapi pod identified by shootNamespace and podName.
	// If the registry does not contain a record for the specified pod, the operation has no effect.
	SetKapiMetrics(shootNamespace string, podName string, currentTotalRequestCount int64)
	// SetKapiLastScrapeTime records the start time of the last scrape for the Kapi pod identified by shootNamespace and podName.
	// If the registry does not contain a record for the specified pod, the operation has no effect.
	SetKapiLastScrapeTime(shootNamespace string, podName string, value time.Time)
	// NotifyKapiMetricsFault is the counterpart of SetKapiMetrics which is used when a metrics scrape fails. Instead of
	// recording the newly obtained metrics values, it records the fact that values could not be obtained.
	// If the registry does not contain a record for the specified pod, the operation has no effect.
	//
	// The function returns the number of consecutive faults on record, including the one reflected by this call.
	// Returns -1 if the registry currently does not maintain a record for the specified pod.
	NotifyKapiMetricsFault(shootNamespace string, podName string) int
	// GetShootAuthSecret retrieves the authentication secret used to access Kapi metrics on the shoot identified by shootNamespace.
	// Returns empty string if there is no auth secret on record for that shoot.
	GetShootAuthSecret(shootNamespace string) string
	// SetShootAuthSecret records the specified authentication secret for the shoot identified by ShootNamespace, so it can
	// later be retrieved via GetShootAuthSecret(). Passing authSecret="" deletes the record, if one exists.
	SetShootAuthSecret(shootNamespace string, authSecret string)
	// GetShootCACertificate retrieves the Kapi CA certificate registered for the shoot identified by shootNamespace.
	// Returns nil if a CA cert is not registered for the shoot. The result is in the form of a CertPool, containing
	// only the shoot's CA certificate. Callers should not modify the returned object.
	GetShootCACertificate(shootNamespace string) *x509.CertPool
	// SetShootCACertificate records the specified certificate as the CA certificate for the Kapi of the shoot identified by
	// shootNamespace, so it can later be retrieved via GetShootCACertificate(). Passing certificate=nil deletes the record,
	// if one exists.
	SetShootCACertificate(shootNamespace string, certificate []byte)
	// AddKapiWatcher subscribes an event handler which gets called when there is a change in the ShootKapi objects on
	// record in the registry.
	// If shouldNotifyOfPreexisting is true, a KapiEventCreate event will be delivered to the watcher for each ShootKapi
	// which is already in the registry at the time of the call. If false, the watcher will only be notified of subsequent
	// changes.
	//
	// IMPORTANT:
	// If a goroutine exists which could hold a given lock while calling a method on a given InputDataRegistry instance,
	// then it is illegal for any KapiWatcher registered on that instance to block, even indirectly, on that same lock.
	// The KapiWatcher is still allowed to e.g. create a separate goroutine which blocks in the lock, as long as it doesn't
	// block waiting on the goroutine.
	AddKapiWatcher(watcher *KapiWatcher, shouldNotifyOfPreexisting bool)
	// RemoveKapiWatcher removes the event watcher, registered by a prior AddKapiWatcher call.
	// The watcher pointer must have the same value as the one provided to said AddKapiWatcher() call.
	// Returns false, if the specified watcher has never been added to the registry, or was already removed.
	RemoveKapiWatcher(watcher *KapiWatcher) bool
}

// InputDataRegistry holds data based on kube-apiserver application metrics and information necessary to scrape such
// metrics. The scope of one instance is multiple shoots on the same seed. All public operations are concurrency-safe.
type inputDataRegistry struct {
	// See MinSampleGap in input.CLIConfig
	minSampleGap time.Duration
	// Maps <shoot namespace> -> <shootData object>. Values cannot be null.
	shoots map[string]*shootData

	// Synchronizes access to all fields of the type.
	lock sync.Mutex

	// Records all subscribers who expressed interest in Kapi change notifications.
	// Note that closures cannot be compared for equality but pointers to closure can, so subscriber closures are
	// represented by a pointer. Client code is responsible for sending the exact same pointer back, when requesting
	// that a subscription be terminated.
	kapiWatchers []*KapiWatcher
	log          logr.Logger

	testIsolation inputDataRegistryTestIsolation // Provides indirections necessary to isolate the unit during tests
}

// NewInputDataRegistry creates a new InputDataRegistry object
func NewInputDataRegistry(minSampleGap time.Duration, log logr.Logger) InputDataRegistry {
	return &inputDataRegistry{
		minSampleGap: minSampleGap,
		shoots:       make(map[string]*shootData),
		log:          log,
		testIsolation: inputDataRegistryTestIsolation{
			TimeNow: time.Now,
		},
	}
}

// DataSource returns an InputDataSource interface to the registry, which is focused on metrics consumption, and
// abstracts other details away.
func (reg *inputDataRegistry) DataSource() InputDataSource {
	return &dataSourceAdapter{reg}
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Individual pod operations

// getKapiDataThreadUnsafe returns a reference (not copy) to the respective KapiData in the registry, or nil
func (reg *inputDataRegistry) getKapiDataThreadUnsafe(shootNamespace string, podName string) *KapiData {
	shoot := reg.shoots[shootNamespace]
	if shoot == nil {
		return nil
	}

	kapiIndex := slices.IndexFunc(shoot.KapiData, func(x *KapiData) bool { return x.PodName() == podName })
	if kapiIndex == -1 { // Not found
		return nil
	}

	return shoot.KapiData[kapiIndex]
}

// GetKapiData returns a KapiData object which contains the registry's information, specific to the Kapi pod identified
// by shootNamespace and podName.
// The output is a deep copy, and fully detached from the registry. If the registry has no information about the
// specified pod, nil is returned.
func (reg *inputDataRegistry) GetKapiData(shootNamespace string, podName string) *KapiData {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	pkapi := reg.getKapiDataThreadUnsafe(shootNamespace, podName)

	if pkapi == nil {
		return nil
	}
	result := *pkapi
	return &result
}

// SetKapiData stores registry data specific to the k8s Kapi pod object identified by shootNamespace and podName.
func (reg *inputDataRegistry) SetKapiData(
	shootNamespace string, podName string, podUID types.UID, podLabels map[string]string, metricsUrl string) {

	reg.lock.Lock()
	defer reg.lock.Unlock()

	kapi, isCreate := reg.getOrCreateKapiDataThreadUnsafe(shootNamespace, podName)
	kapi.PodUID = podUID
	kapi.MetricsUrl = metricsUrl
	kapi.PodLabels = podLabels
	if isCreate {
		reg.notifyKapiWatchersThreadUnsafe(kapi, KapiEventCreate)
	}
}

// RemoveKapiData deletes all registry data specific to the Kapi pod identified by shootNamespace and podName.
// The output value is false if the registry did not contain data for the identified pod.
func (reg *inputDataRegistry) RemoveKapiData(shootNamespace string, podName string) bool {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	shoot := reg.shoots[shootNamespace]
	if shoot == nil {
		return false
	}

	kapiIndex := slices.IndexFunc(shoot.KapiData, func(x *KapiData) bool { return x.PodName() == podName })
	if kapiIndex == -1 { // Not found
		return false
	}

	// Raise event just before deleting
	reg.notifyKapiWatchersThreadUnsafe(shoot.KapiData[kapiIndex], KapiEventDelete)

	// Are we removing the last piece of information?
	if len(shoot.KapiData) == 1 {
		if shoot.AuthSecret == "" && shoot.CACertPool == nil {
			// No more data in the KapiData object, just remove from registry
			delete(reg.shoots, shootNamespace)
			return true
		}

		// Removing the last KapiData for the shoot, just drop the slice
		shoot.KapiData = nil
		return true
	}

	shoot.KapiData = append(shoot.KapiData[:kapiIndex], shoot.KapiData[kapiIndex+1:]...)
	return true
}

// SetKapiMetrics records the current metrics value for the Kapi pod identified by shootNamespace and podName.
// If the registry does not contain a record for the specified pod, the operation has no effect.
func (reg *inputDataRegistry) SetKapiMetrics(shootNamespace string, podName string, currentTotalRequestCount int64) {
	now := reg.testIsolation.TimeNow()
	reg.lock.Lock()
	defer reg.lock.Unlock()

	kapi := reg.getKapiDataThreadUnsafe(shootNamespace, podName)
	if kapi == nil {
		return
	}

	kapi.FaultCount = 0
	if currentTotalRequestCount < kapi.TotalRequestCountNew || // Sample is out of order
		now.Sub(kapi.MetricsTimeNew) < reg.minSampleGap { // Scraped too soon, poor differentiation accuracy

		return
	}

	kapi.MetricsTimeOld = kapi.MetricsTimeNew
	kapi.TotalRequestCountOld = kapi.TotalRequestCountNew
	kapi.MetricsTimeNew = now
	kapi.TotalRequestCountNew = currentTotalRequestCount
	reg.log.V(app.VerbosityVerbose).
		WithValues("ns", shootNamespace, "name", podName, "requestCount", kapi.TotalRequestCountNew).
		Info("New total request count for kapi")
}

// SetKapiLastScrapeTime records the start time of the last scrape for the Kapi pod identified by shootNamespace and podName.
// If the registry does not contain a record for the specified pod, the operation has no effect.
func (reg *inputDataRegistry) SetKapiLastScrapeTime(shootNamespace string, podName string, value time.Time) {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	kapi := reg.getKapiDataThreadUnsafe(shootNamespace, podName)
	if kapi == nil {
		return
	}

	kapi.LastMetricsScrapeTime = value
}

// NotifyKapiMetricsFault is the counterpart of SetKapiMetrics which is used when a metrics scrape fails. Instead of
// recording the newly obtained metrics values, it records the fact that values could not be obtained.
// If the registry does not contain a record for the specified pod, the operation has no effect.
//
// The function returns the number of consecutive faults on record, including the one reflected by this call.
// Returns -1 if the registry currently does not maintain a record for the specified pod.
func (reg *inputDataRegistry) NotifyKapiMetricsFault(shootNamespace string, podName string) int {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	kapi := reg.getKapiDataThreadUnsafe(shootNamespace, podName)
	if kapi == nil {
		return -1
	}

	kapi.FaultCount++
	return kapi.FaultCount
}

// Caller must acquire write lock before calling this function
// Returns:
// - Pointer to the resulting KapiData
// - A bool: Was the KapiData created, or did it already exist. True means "created".
func (reg *inputDataRegistry) getOrCreateKapiDataThreadUnsafe(shootNamespace string, podName string) (*KapiData, bool) {
	shoot := reg.getOrCreateShootDataThreadUnsafe(shootNamespace)
	kapiIndex := slices.IndexFunc(shoot.KapiData, func(x *KapiData) bool { return x.PodName() == podName })

	if kapiIndex != -1 { // Already exists
		return shoot.KapiData[kapiIndex], false
	}

	kapi := &KapiData{shootNamespace: shootNamespace, podName: podName}
	shoot.KapiData = append(shoot.KapiData, kapi)
	return kapi, true
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Shoot operations

// GetShootAuthSecret retrieves the authentication secret used to access Kapi metrics on the shoot identified by shootNamespace.
// Returns empty string if there is no auth secret on record for that shoot.
func (reg *inputDataRegistry) GetShootAuthSecret(shootNamespace string) string {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	shoot := reg.shoots[shootNamespace]

	if shoot == nil {
		return ""
	}

	return shoot.AuthSecret
}

// SetShootAuthSecret records the specified authentication secret for the shoot identified by ShootNamespace, so it can
// later be retrieved via GetShootAuthSecret(). Passing authSecret="" deletes the record, if one exists.
func (reg *inputDataRegistry) SetShootAuthSecret(shootNamespace string, authSecret string) {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	shoot := reg.shoots[shootNamespace]

	if shoot == nil {
		if authSecret == "" {
			// There's nothing to remove. Just return.
			return
		}

		shoot = &shootData{shootNamespace: shootNamespace}
		reg.shoots[shootNamespace] = shoot
	} else {
		// Was this the last piece of information for that shoot?
		if authSecret == "" && shoot.CACertPool == nil && shoot.KapiData == nil {
			delete(reg.shoots, shootNamespace)
			return
		}
	}

	shoot.AuthSecret = authSecret
}

// GetShootCACertificate retrieves the Kapi CA certificate registered for the shoot identified by shootNamespace.
// Returns nil if a CA cert is not registered for the shoot. The result is in the form of a CertPool, containing
// only the shoot's CA certificate. Callers should not modify the returned object.
func (reg *inputDataRegistry) GetShootCACertificate(shootNamespace string) *x509.CertPool {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	shoot := reg.shoots[shootNamespace]
	if shoot == nil {
		return nil
	}

	return shoot.CACertPool
}

// SetShootCACertificate records the specified certificate as the CA certificate for the Kapi of the shoot identified by
// shootNamespace, so it can later be retrieved via GetShootCACertificate(). Passing certificate=nil deletes the record,
// if one exists.
func (reg *inputDataRegistry) SetShootCACertificate(shootNamespace string, certificate []byte) {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	shoot := reg.shoots[shootNamespace]

	if shoot == nil {
		if certificate == nil {
			// There's nothing to remove. Just return.
			return
		}

		shoot = &shootData{shootNamespace: shootNamespace}
		reg.shoots[shootNamespace] = shoot
	} else {
		// Was this the last piece of information for that shoot?
		if certificate == nil && shoot.AuthSecret == "" && shoot.KapiData == nil {
			delete(reg.shoots, shootNamespace)
			return
		}
	}

	if certificate == nil {
		shoot.CACertPool = nil
		return
	}

	shoot.CACertPool = x509.NewCertPool()
	shoot.CACertPool.AppendCertsFromPEM(certificate)
}

// Caller must acquire write lock before calling this function
func (reg *inputDataRegistry) getOrCreateShootDataThreadUnsafe(shootNamespace string) *shootData {
	shoot := reg.shoots[shootNamespace]

	if shoot == nil {
		shoot = &shootData{
			shootNamespace: shootNamespace,
		}
		reg.shoots[shootNamespace] = shoot
	}

	return shoot
}

//#region Events

// AddKapiWatcher subscribes an event handler which gets called when there is a change in the ShootKapi objects on
// record in the registry.
// If shouldNotifyOfPreexisting is true, a KapiEventCreate event will be delivered to the watcher for each ShootKapi
// which is already in the registry at the time of the call. If false, the watcher will only be notified of subsequent
// changes.
//
// IMPORTANT:
// If a goroutine exists which could hold a given lock while calling a method on a given InputDataRegistry instance,
// then it is illegal for any KapiWatcher registered on that instance to block, even indirectly, on that same lock.
// The KapiWatcher is still allowed to e.g. create a separate goroutine which blocks in the lock, as long as it doesn't
// block waiting on the goroutine.
func (reg *inputDataRegistry) AddKapiWatcher(watcher *KapiWatcher, shouldNotifyOfPreexisting bool) {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	if shouldNotifyOfPreexisting {
		for _, shoot := range reg.shoots {
			for _, kapi := range shoot.KapiData {
				(*watcher)(&kapiDataAdapter{x: kapi}, KapiEventCreate)
			}
		}
	}

	reg.kapiWatchers = append(reg.kapiWatchers, watcher)
}

// RemoveKapiWatcher removes the event watcher, registered by a prior AddKapiWatcher call.
// The watcher pointer must have the same value as the one provided to said AddKapiWatcher() call.
// Returns false, if the specified watcher has never been added to the registry, or was already removed.
func (reg *inputDataRegistry) RemoveKapiWatcher(watcher *KapiWatcher) bool {
	reg.lock.Lock()
	defer reg.lock.Unlock()

	for i, value := range reg.kapiWatchers {
		if value == watcher {
			reg.kapiWatchers = append(reg.kapiWatchers[:i], reg.kapiWatchers[i+1:]...)
			return true
		}
	}

	return false
}

// Caller must acquire read lock before calling this function
func (reg *inputDataRegistry) notifyKapiWatchersThreadUnsafe(kapi *KapiData, event KapiEventType) {
	for _, watcher := range reg.kapiWatchers {
		(*watcher)(&kapiDataAdapter{x: kapi}, event)
	}
}

//#endregion Events

//#region Test isolation

// inputDataRegistryTestIsolation contains all points of indirection necessary to isolate static function calls
// in the InputDataRegistry unit during tests
type inputDataRegistryTestIsolation struct {
	// Points to [time.Now]
	TimeNow func() time.Time
}

//#endregion Test isolation
