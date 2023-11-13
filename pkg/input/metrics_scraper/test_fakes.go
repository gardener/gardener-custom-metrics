//nolint:all
package metrics_scraper

import (
	"context"
	"crypto/x509"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

//#region fakeScrapeQueue

type fakeScrapeQueue struct {
	Registry     input_data_registry.InputDataRegistry
	Queue        []*scrapeTarget
	isClosed     bool
	ScrapePeriod time.Duration
	IsNoRequeue  bool // If true, GetNext() permanently dequeues the head, instead re-queuing it on the back
	lock         sync.Mutex
}

func newFakeScrapeQueue(registry input_data_registry.InputDataRegistry, scrapePeriod time.Duration) *fakeScrapeQueue {
	return &fakeScrapeQueue{Registry: registry, ScrapePeriod: scrapePeriod}
}

func (fsq *fakeScrapeQueue) GetNext() *scrapeTarget {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	if len(fsq.Queue) == 0 {
		return nil
	}
	head := fsq.Queue[0]
	if fsq.IsNoRequeue {
		fsq.Queue = fsq.Queue[1:]
	} else {
		fsq.Queue = append(fsq.Queue[1:], head)
	}
	return head
}

func (fsq *fakeScrapeQueue) Count() int {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	return len(fsq.Queue)
}

func (fsq *fakeScrapeQueue) DueCount(dueAtTime time.Time, excludeUnscraped bool) int {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	dueCount := 0
	for _, target := range fsq.Queue {
		kapi := fsq.Registry.GetKapiData(target.Namespace, target.PodName)
		if excludeUnscraped && (kapi.LastMetricsScrapeTime == time.Time{}) {
			continue
		}
		if kapi.LastMetricsScrapeTime.Add(fsq.ScrapePeriod).After(dueAtTime) {
			break
		}
		dueCount++
	}
	return dueCount
}

func (fsq *fakeScrapeQueue) Close() (err error) {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	fsq.isClosed = true
	return nil
}

func (fsq *fakeScrapeQueue) IsClosed() bool {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	return fsq.isClosed
}

func (fsq *fakeScrapeQueue) EmptyQueue() {
	fsq.lock.Lock()
	defer fsq.lock.Unlock()

	fsq.Queue = nil
}

//#endregion fakeScrapeQueue

// scraperTestMetrics stores metrics which are recorded during the action phase of unit tests, and examined during
// the assertion phase
type scraperTestMetrics struct {
	WorkerProcCount atomic.Int32
}

//#region fakeMetricsClient

type fakeMetricsClient struct {
	WasScraped          atomic.Bool
	lastContextDuration atomic.Int64
}

const fakeMetricsClientMetricsValue int64 = 777

// GetLastContextDuration returns an approximation of the duration constraint of the context passed to the last
// GetKapiInstanceMetrics call. The value is inaccurate, because contexts have a deadline, instead of duration.
// The duration is deduced, based on an assumption that test execution takes negligible time. If the validity of that
// assumption is broken (e.g. by a debugger breakpoint mid-test), the returned value will be incorrect.
func (mc *fakeMetricsClient) GetLastContextDuration() time.Duration {
	return time.Duration(mc.lastContextDuration.Load())
}

func (mc *fakeMetricsClient) GetKapiInstanceMetrics(ctx context.Context, _ string, _ string, _ *x509.CertPool) (result int64, err error) {
	if deadline, ok := ctx.Deadline(); ok {
		mc.lastContextDuration.Store(int64(deadline.Sub(time.Now()))) // Assumes instantaneous test execution
	} else {
		mc.lastContextDuration.Store(0)
	}
	mc.WasScraped.Store(true)
	return fakeMetricsClientMetricsValue, nil
}

//#endregion fakeMetricsClient
