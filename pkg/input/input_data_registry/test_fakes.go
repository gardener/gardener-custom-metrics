//nolint:all
package input_data_registry

import (
	"crypto/x509"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type FakeInputDataRegistry struct {
	authSecret                       string
	HasNoCACertificate               bool
	Watcher                          *KapiWatcher
	ShouldWatcherNotifyOfPreexisting bool
	kapis                            []*KapiData
	lock                             sync.Mutex

	MinSampleGap time.Duration
}

func (fidr *FakeInputDataRegistry) GetKapis() []*KapiData {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	result := make([]*KapiData, len(fidr.kapis))
	for i, kapi := range fidr.kapis {
		result[i] = kapi.Copy()
	}
	return result
}

func (fidr *FakeInputDataRegistry) SetKapis(kapis []*KapiData) {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	fidr.kapis = kapis
}

func (fidr *FakeInputDataRegistry) DataSource() InputDataSource {
	return &fakeDataSourceAdapter{fidr}
}

func (fidr *FakeInputDataRegistry) getKapiDataThreadUnsafe(shootNamespace string, podName string) *KapiData {
	for _, kapi := range fidr.kapis {
		if kapi.shootNamespace == shootNamespace && kapi.podName == podName {
			return kapi
		}
	}
	return nil
}

func (fidr *FakeInputDataRegistry) GetKapiData(shootNamespace string, podName string) *KapiData {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	return fidr.getKapiDataThreadUnsafe(shootNamespace, podName).Copy()
}

func (fidr *FakeInputDataRegistry) SetKapiData(
	shootNamespace string, podName string, uid types.UID, podLabels map[string]string, metricsUrl string) {

	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	for _, kapi := range fidr.kapis {
		if kapi.shootNamespace == shootNamespace && kapi.podName == podName {
			kapi.MetricsUrl = metricsUrl
			kapi.PodUID = uid
			kapi.PodLabels = podLabels
			return
		}
	}
	fidr.kapis = append(fidr.kapis, &KapiData{
		shootNamespace: shootNamespace,
		podName:        podName,
		PodUID:         uid,
		MetricsUrl:     metricsUrl,
		PodLabels:      podLabels,
	})
}

func (fidr *FakeInputDataRegistry) RemoveKapiData(shootNamespace string, podName string) bool {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	for i, kapi := range fidr.kapis {
		if kapi.shootNamespace == shootNamespace && kapi.podName == podName {
			fidr.kapis = append(fidr.kapis[:i], fidr.kapis[i+1:]...)
			return true
		}
	}
	return false
}

func (fidr *FakeInputDataRegistry) SetKapiMetrics(shootNamespace string, podName string, currentTotalRequestCount int64) {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	fidr.getKapiDataThreadUnsafe(shootNamespace, podName).TotalRequestCountNew = currentTotalRequestCount
}

func (fidr *FakeInputDataRegistry) SetKapiMetricsWithTime(
	shootNamespace string, podName string, currentTotalRequestCount int64, metricsTime time.Time) {

	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	kapi := fidr.getKapiDataThreadUnsafe(shootNamespace, podName)
	kapi.TotalRequestCountOld = kapi.TotalRequestCountNew
	kapi.MetricsTimeOld = kapi.MetricsTimeNew
	kapi.TotalRequestCountNew = currentTotalRequestCount
	kapi.MetricsTimeNew = metricsTime
}

func (fidr *FakeInputDataRegistry) SetKapiLastScrapeTime(shootNamespace string, podName string, value time.Time) {
	fidr.lock.Lock()
	defer fidr.lock.Unlock()

	fidr.getKapiDataThreadUnsafe(shootNamespace, podName).LastMetricsScrapeTime = value
}

func (fidr *FakeInputDataRegistry) NotifyKapiMetricsFault(_ string, _ string) int {
	panic("implement me")
}

func (fidr *FakeInputDataRegistry) GetShootAuthSecret(_ string) string {
	if fidr.authSecret == "" {
		return "auth secret"
	}
	if fidr.authSecret == "__EMPTY__" {
		return ""
	}
	return fidr.authSecret
}

func (fidr *FakeInputDataRegistry) RemoveShootAuthSecret() {
	fidr.authSecret = "__EMPTY__"
}

func (fidr *FakeInputDataRegistry) SetShootAuthSecret(_ string, _ string) {
	panic("implement me")
}

func (fidr *FakeInputDataRegistry) GetShootCACertificate(_ string) *x509.CertPool {
	if fidr.HasNoCACertificate {
		return nil
	}
	return x509.NewCertPool()
}

func (fidr *FakeInputDataRegistry) SetShootCACertificate(_ string, _ []byte) {
	panic("implement me")
}

func (fidr *FakeInputDataRegistry) AddKapiWatcher(watcher *KapiWatcher, shouldNotifyOfPreexisting bool) {
	if fidr.Watcher != nil {
		panic("more than one watchers added")
	}
	fidr.Watcher = watcher
	fidr.ShouldWatcherNotifyOfPreexisting = shouldNotifyOfPreexisting
}

func (fidr *FakeInputDataRegistry) RemoveKapiWatcher(*KapiWatcher) bool {
	if fidr.Watcher == nil {
		return false
	}
	fidr.Watcher = nil
	return true
}

type fakeDataSourceAdapter struct{ x *FakeInputDataRegistry }

func (a *fakeDataSourceAdapter) GetShootKapis(_ string) []ShootKapi {
	a.x.lock.Lock()
	defer a.x.lock.Unlock()

	var result = make([]ShootKapi, len(a.x.kapis))
	for i := range a.x.kapis {
		x := *a.x.kapis[i]
		result[i] = &kapiDataAdapter{&x}
	}

	return result
}

func (a *fakeDataSourceAdapter) AddKapiWatcher(_ *KapiWatcher, _ bool) {
	panic("implement me")
}

func (a *fakeDataSourceAdapter) RemoveKapiWatcher(_ *KapiWatcher) bool {
	panic("implement me")
}
