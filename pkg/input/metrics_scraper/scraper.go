package metrics_scraper

import (
	"context"
	"math"
	"runtime/pprof"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// Scraper tracks the kube-apiserver pods in a [input_data_registry.InputDataRegistry] and populates the registry back
// with metrics scraped from the pods
//
// Remarks:
// The current Scraper implementation is meant for seeds which contain 20-6000 shoot kube-apiserver pods.
// With a much lower number of shoots, operation is functionally correct, but somewhat suboptimal. With a much higher
// number of shoots, contention for some internal synchronisation objects may become a bottleneck.
type Scraper struct {
	// The dataRegistry serves as both a source of input data driving the scraper, and as store for the output data
	// produced by the scraper.
	dataRegistry input_data_registry.InputDataRegistry
	log          logr.Logger

	///////////////////////////////////////////////////////////////////////////
	// Parameters:

	// How often do we adjust the level of parallelism to reflect work load
	scrapeShiftPeriod time.Duration

	// Min number of goprocs (workers) created in a scheduling step (shift)
	minShiftWorkerCount int

	// Max number of goprocs (workers) created in a scheduling step (shift)
	maxShiftWorkerCount int

	// Max number of simultaneous scraping goprocs (workers). Includes leftover workers from current shift and workers
	// from previous shifts
	maxActiveWorkerCount int

	// Abort a scrape request if it takes longer than that
	scrapeTimeout time.Duration

	///////////////////////////////////////////////////////////////////////////
	// Worker scheduling state:

	// Only used by shift scheduler - no need to sync access
	lastShiftStartTime time.Time

	// How many parallel workers did we spawn to scrape last time. Only used by shift scheduler - no need to sync access
	lastShiftWorkerCount int

	// How many Kapis did we aim to scrape last time. Only used by shift scheduler - no need to sync access
	lastShiftScrapeTargetCount int

	// Determines scrape order and timing. No need to sync access - the pointer is immutable, and the public interfafe
	// of a ScrapeQueue is concurrency-safe.
	queue scrapeQueue

	// How many workers are still running
	activeWorkerCount atomic.Int32

	// Tracks the worker goprocs doing the actual scraping
	workerWaitGroup sync.WaitGroup

	// Provides indirections necessary to isolate the unit during tests
	testIsolation scraperTestIsolation
}

// Start implements sigs.k8s.io/controller-runtime/pkg/manager.Runnable. It starts data gathering activities and only
// returns after all such activities have stopped.
//
// Errors which occur during individual scrapes do not terminate the overall scraping process, and are thus not
// reflected in the error returned by this function.
func (s *Scraper) Start(ctx context.Context) error {
	log := s.log.WithValues("op", "scraperProc")

	ticker := s.testIsolation.NewTicker(s.scrapeShiftPeriod)
	log.V(app.VerbosityVerbose).Info("Scraper started", "schedulingPeriod", s.scrapeShiftPeriod)
	defer ticker.Stop()
	defer s.workerWaitGroup.Wait()

loop:
	for {
		select {
		case <-ctx.Done():
			log.V(app.VerbosityInfo).Info("Context closed, exiting")
			if err := s.queue.Close(); err != nil {
				log.V(app.VerbosityError).Info("closing scrape queue: %w", err)
			}
			break loop
		case <-ticker.C():
			s.startShiftWorkers(ctx)
		}
	}

	return nil
}

// A shift is the time slice between two adjustments of the level of scraping parallelism. A shiftScheduleArgs records
// the parameters which affect scheduling in a given shift.
type shiftScheduleArgs struct {
	StartTime   time.Time // Shift start
	TargetCount int       // Scrape target count
	WorkerCount int       // Count of dedicated workers started for this shift
}

// startShiftWorkers estimates the necessary number of worker goroutines for the next shift and starts them.
//
// This function is not reentrant, as it performs unsynchronised access to some receiver fields.
func (s *Scraper) startShiftWorkers(ctx context.Context) {
	log := s.log.WithValues("op", "startShiftWorkers")

	// At this point, there is a conflict as to what the "lastShift..." fields in the Scraper refer to. That is because
	// in addition to the values from the previously completed shift, we also need to calculate new values for the now
	// starting shift, and store them in those same fields. So, there are two valid frames of reference for those
	// fields - one at the start of the current shift, and one at the end of it. We need to get the old values out,
	// and use them to calculate and write the new values.

	// Cache values for the previous frame of reference
	lastShift := shiftScheduleArgs{
		StartTime:   s.lastShiftStartTime,
		TargetCount: s.lastShiftScrapeTargetCount,
		WorkerCount: s.lastShiftWorkerCount,
	}
	// Allocate a place where we'll store values for the new frame of reference. We'll apply these later.
	now := s.testIsolation.TimeNow()
	thisShift := shiftScheduleArgs{
		StartTime:   now,
		TargetCount: s.queue.DueCount(now, false),
		WorkerCount: -1, // We'll calculate this one shortly
	}

	// How many from last shift have not even been picked for processing. We don't count targets which have never been
	// scraped. Chances are, they were added after last shift ended.
	lastShiftUnprocessedCount := s.queue.DueCount(lastShift.StartTime, true)
	lastShiftWorkerThroughput := float64(lastShift.TargetCount-lastShiftUnprocessedCount) / float64(lastShift.WorkerCount)
	if lastShiftWorkerThroughput < 1 {
		// A worker is practically guaranteed to pick at least one target. So, if we're getting throughput < 1, that's
		// because last shift had targets < workers. In that case, use the guaranteed min throughput.
		lastShiftWorkerThroughput = 1
	}

	log.V(app.VerbosityVerbose).Info("Shift begins",
		"lastStart", lastShift.StartTime,
		"lastTargets", lastShift.TargetCount,
		"lastWorkers", lastShift.WorkerCount,
		"leftovers", lastShiftUnprocessedCount,
		"thisStart", thisShift.StartTime,
		"thisTargets", thisShift.TargetCount)

	if lastShiftUnprocessedCount > 0 {
		// Estimate how many workers we need in this shift, assuming individual worker's throughput same as last shift.
		// Note that under provisioning workers is not an issue, because workers from previous shifts, who happen
		// to still be in when this shift begins, are not allowed to leave until this shift's work is done.
		thisShift.WorkerCount = int(math.Ceil(float64(thisShift.TargetCount) / lastShiftWorkerThroughput))
		if thisShift.WorkerCount > 2*lastShift.WorkerCount {
			// The most growth we allow across two consecutive shifts, is doubling the workers. There are better
			// algorithms, but this one is simpler and less error-prone.
			thisShift.WorkerCount = 2 * lastShift.WorkerCount
		}
	} else {
		// To be safe, we don't reduce workers based on throughput estimates. Instead, we slowly decay the worker count
		thisShift.WorkerCount = lastShift.WorkerCount - 1
	}

	if thisShift.WorkerCount < s.minShiftWorkerCount {
		thisShift.WorkerCount = s.minShiftWorkerCount
	} else {
		if thisShift.WorkerCount > s.maxShiftWorkerCount {
			thisShift.WorkerCount = s.maxShiftWorkerCount
		}
		allowedPerTotalMax := s.maxActiveWorkerCount - int(s.activeWorkerCount.Load())
		if thisShift.WorkerCount > allowedPerTotalMax {
			thisShift.WorkerCount = allowedPerTotalMax
		}
	}

	// Move frame of reference to current shift
	s.lastShiftStartTime = thisShift.StartTime
	s.lastShiftScrapeTargetCount = thisShift.TargetCount
	s.lastShiftWorkerCount = thisShift.WorkerCount

	log.V(app.VerbosityVerbose).Info("Starting workers", "count", thisShift.WorkerCount)
	for i := 0; i < thisShift.WorkerCount; i++ {
		s.workerWaitGroup.Add(1)
		s.activeWorkerCount.Add(1)
		go s.testIsolation.workerProc(ctx)
	}
}

// workerProc is the entry point for a worker goroutine. It scrapes the scrapeQueue until there are no more targets
// eligible for an immediate scrape. The workers are stateless - it makes no functional difference, which worker will
// pick which target for scraping.
func (s *Scraper) workerProc(ctx context.Context) {
	defer s.workerWaitGroup.Done()
	defer s.activeWorkerCount.Add(-1)

	labels := pprof.Labels("workerProc", "")
	pprof.Do(ctx, labels, func(ctx context.Context) {
		s.ScrapeQueue(ctx)
	})
}

// ScrapeQueue sequentially picks targets from the queue and scrapes them, until there are no more eligible targets.
func (s *Scraper) ScrapeQueue(ctx context.Context) {
	for target := s.queue.GetNext(); target != nil && ctx.Err() == nil; target = s.queue.GetNext() {
		s.scrape(ctx, target)
	}
}

// Scrape scrapes metrics from the specified ShootKapi pod and stores them in the Scraper's data registry.
// Errors are not reported by the function. Instead, the failed scrape iteration of that target is just skipped, and
// scrape data becomes temporarily stale, until a subsequent scrape of the same target succeeds.
func (s *Scraper) scrape(ctx context.Context, target *scrapeTarget) {
	log := s.log.WithValues("op", "scrape", "namespace", target.Namespace, "pod", target.PodName)
	kapi := s.dataRegistry.GetKapiData(target.Namespace, target.PodName)
	if kapi == nil {
		log.V(app.VerbosityError).Error(nil, "No record for this Kapi in the registry")
		return
	}
	authToken := s.dataRegistry.GetShootAuthSecret(target.Namespace)
	if authToken == "" {
		log.V(app.VerbosityError).Error(nil, "No secret for this shoot in the registry")
		return
	}
	caCert := s.dataRegistry.GetShootCACertificate(target.Namespace)
	if caCert == nil {
		log.V(app.VerbosityError).Error(nil, "No CA cert for this shoot in the registry")
		return
	}

	timeoutContext, cancel := context.WithTimeout(ctx, s.scrapeTimeout)
	defer cancel()
	totalRequestCount, err := s.testIsolation.NewMetricsClient().GetKapiInstanceMetrics(timeoutContext, kapi.MetricsUrl, authToken, caCert)
	if err != nil {
		consecutiveFaultCount := s.dataRegistry.NotifyKapiMetricsFault(target.Namespace, target.PodName)
		message := "Kapi metrics retrieval failed"
		if consecutiveFaultCount&(consecutiveFaultCount-1) == 0 { // Is it a power of 2? Exponential backoff on errors.
			log.V(app.VerbosityError).Error(err, message)
		} else {
			log.V(app.VerbosityVerbose).Info(message)
		}
		return
	}
	log.V(app.VerbosityVerbose).Info("Request count scraped", "totalRequestCount", totalRequestCount)
	s.dataRegistry.SetKapiMetrics(target.Namespace, target.PodName, totalRequestCount)
}

//#region Test isolation

type ticker interface {
	C() <-chan time.Time
	Stop()
}

// tickerAdapter adapts [time.Ticker] to the ticker interface.
type tickerAdapter struct {
	ticker *time.Ticker
}

func (t *tickerAdapter) Stop() {
	t.ticker.Stop()
}

func (t *tickerAdapter) C() <-chan time.Time {
	return t.ticker.C
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{Channel: make(chan time.Time)}
}

// fakeTicker provides a test fake implementation for the ticker interface. Use newFakeTicker to create instances.
type fakeTicker struct {
	Period  atomic.Int64
	Channel chan time.Time
}

func (ft *fakeTicker) C() <-chan time.Time {
	return ft.Channel
}

func (ft *fakeTicker) Stop() {
}

// scraperTestIsolation contains all points of indirection necessary to isolate static function calls
// in the Scraper unit during tests
type scraperTestIsolation struct {
	// Points to [time.Now]
	TimeNow func() time.Time
	// Points to [newMetricsClient]
	NewMetricsClient func() metricsClient
	// Points to time.NewTicker
	NewTicker func(duration time.Duration) ticker
	// Points to workerProc
	workerProc func(ctx context.Context)
}

//#endregion Test isolation

//#region scraperFactory

// NewScraper creates a new Scraper object which tracks the kube-apiserver pods in the specified dataRegistry and
// populates the registry back with metrics scraped from the pods.
//
// scrapePeriodMilliseconds is how often the same pod will be scraped.
// scrapeFlowControlPeriodMilliseconds is how often the Scraper will adjust the number of parallel workers responsible
// for the actual pod scraping.
func NewScraper(
	dataRegistry input_data_registry.InputDataRegistry,
	scrapePeriod time.Duration,
	scrapeFlowControlPeriod time.Duration,
	log logr.Logger) *Scraper {

	scraper := &Scraper{
		dataRegistry:         dataRegistry,
		queue:                newScrapeQueueFactory().NewScrapeQueue(dataRegistry, scrapePeriod, log.V(1).WithName("queue")),
		log:                  log,
		lastShiftWorkerCount: 1, // Avoid division by zero
		// Parameters:
		scrapeShiftPeriod:    scrapeFlowControlPeriod,
		minShiftWorkerCount:  1,
		maxShiftWorkerCount:  10,
		maxActiveWorkerCount: 50,

		// Longer timeout increases tolerance to intermittent disruptions and server overload.
		// On the downside:
		// - It creates a risk that a delayed sample and the one after it are too close and hurt impact
		// differential (rate) calculation accuracy.
		// - Allows unresponsive server to tie more resources (active goroutines) on our side.
		scrapeTimeout: scrapePeriod / 2,

		testIsolation: scraperTestIsolation{
			TimeNow:          time.Now,
			NewMetricsClient: newMetricsClient,
			NewTicker: func(period time.Duration) ticker {
				return &tickerAdapter{ticker: time.NewTicker(period)}
			},
		},
	}
	scraper.testIsolation.workerProc = scraper.workerProc

	return scraper
}

//#endregion scraperFactory
