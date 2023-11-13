package metrics_scraper

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

// scrapeTarget identifies a pod in a [input_data_registry.InputDataRegistry] as target for metrics scraping
type scrapeTarget struct {
	Namespace string
	PodName   string
}

// kapiEvent holds information pertaining to a Kapi update event, for the purpose of asynchronous event processing
type kapiEvent struct {
	Namespace string
	PodName   string
	EventType input_data_registry.KapiEventType
}

type scrapeQueue interface {
	// GetNext returns the next target eligible for immediate scraping. If no targets are eligible at the present
	// moment, it returns nil.
	//
	// Criteria to scrape a target (any of the following):
	// - Scrape period interval elapsed since the last time the target was scraped
	// - A scrape is required to maintain the queue's desired minimum scrape rate
	GetNext() *scrapeTarget
	// Count returns the number of targets in the queue
	Count() int
	// DueCount counts the targets for which a scrape would be due (including overdue), at the specified time, per
	// current state of the queue.
	DueCount(dueAtTime time.Time, excludeUnscraped bool) int
	// Close terminates this scrapeQueueImpl's subscription to [input_data_registry.InputDataRegistry] events.
	//
	// Remarks:
	// The queue does not respond to events which occur after Close() returns. However, Close() may return while a past
	// event is still being processing. I.e, Close() guarantees that internal queue activities will eventually seize,
	// but not that they have seized.
	Close() (err error)
}

// scrapeQueue prescribes an order and timing for scraping the pods in a [input_data_registry.InputDataRegistry].
// It tracks the state of the [input_data_registry.InputDataRegistry] by subscribing for events.
//
// Scraping is governed by a configurable scraping period. It progresses at a default rate of ScrapePeriod/TargetCount.
// If for some reason scraping is delayed from that default schedule, it temporarily switches to a higher rate, until
// it catches up.
//
// Public members are concurrency-safe.
type scrapeQueueImpl struct {
	targets     *list.List                            // That's the queue proper, reflecting the scrape order
	registry    input_data_registry.InputDataRegistry // scrapeQueueImpl does not cache pod data. It fetches it from the registry when needed.
	pacemaker   pacemaker                             // Determines the scrape timing, based on rate/burst settings
	kapiWatcher input_data_registry.KapiWatcher       // The event handler subscribed for data events
	log         logr.Logger

	// Synchronizes access to targets. The kapiWatcher should not acquire this lock during its invocation (see
	// [input_data_registry.InputDataRegistry.AddKapiWatcher]).
	targetLock sync.Mutex

	// Mediates Kapi update events, for delayed asynchronous processing, preserving order.
	updateQueue     chan *kapiEvent
	updateQueueLock sync.Mutex

	// How long before all targets are scraped, and we get back to scraping the same target again
	scrapePeriod time.Duration

	testIsolation scrapeQueueTestIsolation // Provides indirections necessary to isolate the unit during tests
}

// getNextCandidateThreadUnsafe returns the next target from the head of the queue, plus its respective Kapi from the
// registry. It returns (nil, nil) if there are no suitable targets on the queue. If the target in front of queue is
// missing from the registry it removes it from the queue and proceeds to try the next target.
//
// The caller must acquire the targetLock before calling this method.
func (q *scrapeQueueImpl) getNextCandidateThreadUnsafe(
	log logr.Logger) (currentTarget *scrapeTarget, kapi *input_data_registry.KapiData) {

	for {
		if q.targets.Len() == 0 {
			log.V(app.VerbosityVerbose).Info("Queue already empty.")
			return nil, nil
		}

		currentTarget = q.targets.Front().Value.(*scrapeTarget)
		kapi = q.registry.GetKapiData(currentTarget.Namespace, currentTarget.PodName)
		if kapi != nil {
			// We have our target and kapi
			return currentTarget, kapi
		}

		// Target was removed from the registry, but the remove notification has not yet been acted upon. Remove from
		// queue and continue with next target on the queue.
		log.WithValues("namespace", currentTarget.Namespace, "pod", currentTarget.PodName).
			V(app.VerbosityInfo).Info("The target is in the scrape queue but missing from the registry.")
		q.targets.Remove(q.targets.Front())
	}
}

func (q *scrapeQueueImpl) GetNext() *scrapeTarget {
	log := q.log.WithValues("op", "GetNext")
	q.targetLock.Lock()
	defer q.targetLock.Unlock()

	currentTarget, kapi := q.getNextCandidateThreadUnsafe(log)
	if currentTarget == nil {
		return nil
	}
	log = log.WithValues("namespace", currentTarget.Namespace, "pod", currentTarget.PodName)

	// Act based on time
	lastScrapeTime := kapi.LastMetricsScrapeTime
	nextScrapeTime := lastScrapeTime.Add(q.scrapePeriod)
	now := q.testIsolation.TimeNow()
	eagerToProcess := !now.Before(nextScrapeTime) // If it's due time, or past due time, we're eager to scrape
	log = log.WithValues("namespace", currentTarget.Namespace, "pod", currentTarget.PodName)
	log.V(app.VerbosityVerbose).Info("Candidate target selected.", "lastScrape", lastScrapeTime, "eager", eagerToProcess, "now", now)

	if !q.pacemaker.GetScrapePermission(eagerToProcess) {
		log.V(app.VerbosityVerbose).Info("Refused by pacemaker.")
		return nil
	}

	// It's settled: the target will be scraped now
	q.registry.SetKapiLastScrapeTime(currentTarget.Namespace, currentTarget.PodName, now)
	log.V(app.VerbosityVerbose).Info("Target rescheduled.")
	q.targets.MoveToBack(q.targets.Front())
	return currentTarget
}

// onKapiUpdated responds to [input_data_registry.InputDataSource] events, updating the target list and background
// scrape rate
func (q *scrapeQueueImpl) onKapiUpdated(shootKapi input_data_registry.ShootKapi, eventType input_data_registry.KapiEventType) {
	q.updateQueueLock.Lock()
	defer q.updateQueueLock.Unlock()

	// Queue the data, so it can be asynchronously used by the goroutine below. See [input_data_registry.KapiWatcher].
	if q.updateQueue != nil {
		q.updateQueue <- &kapiEvent{shootKapi.ShootNamespace(), shootKapi.PodName(), eventType}
	}
}

// Count returns the number of targets in the queue
func (q *scrapeQueueImpl) Count() int {
	q.targetLock.Lock()
	defer q.targetLock.Unlock()

	return q.targets.Len()
}

func (q *scrapeQueueImpl) DueCount(dueAtTime time.Time, excludeUnscraped bool) int {
	// Targets become due for scraping at the moment when one scrape period elapses from their last scrape
	lastScrapeCutoffTime := dueAtTime.Add(-q.scrapePeriod)
	q.targetLock.Lock()
	defer q.targetLock.Unlock()
	count := 0

	for element := q.targets.Front(); element != nil; element = element.Next() {
		target := element.Value.(*scrapeTarget)
		kapi := q.registry.GetKapiData(target.Namespace, target.PodName)
		if kapi == nil {
			continue // Was removed from the registry, but the removal notification not processed yet. Act as if removed.
		}

		if kapi.LastMetricsScrapeTime.After(lastScrapeCutoffTime) {
			return count
		}

		if !excludeUnscraped || !kapi.LastMetricsScrapeTime.IsZero() {
			count++
		}
	}

	return count
}

func (q *scrapeQueueImpl) Close() (err error) {
	if !q.registry.RemoveKapiWatcher(&q.kapiWatcher) { // Must pass the same address as when adding
		err = fmt.Errorf("close scrape queue: remove data watcher: the queue was not registered as watcher")
	}

	q.updateQueueLock.Lock()
	defer q.updateQueueLock.Unlock()
	if q.updateQueue != nil {
		close(q.updateQueue)
		q.updateQueue = nil
	}
	return
}

// processKapiEvents executes all of a scrapeQueueImpl's ongoing activities. It only returns after all such activities have stopped.
//
// It acts on Kapi update event asynchronously, so the event handler (onKapiUpdated) can return without
// directly acquiring the scrapeQueueImpl.targetLock.
//
// See scrapeQueueImpl.targetLock.
func (q *scrapeQueueImpl) processKapiEvents() {
	q.updateQueueLock.Lock()
	queue := q.updateQueue
	q.updateQueueLock.Unlock()

	if queue == nil {
		return
	}

	// Run Kapi updates asynchronously, so onKapiUpdated can return without directly acquiring the scrapeQueueImpl.targetLock.
	// See scrapeQueueImpl.targetLock.
	for event := range queue {
		q.processSingleKapiEvent(event)
	}
}

func (q *scrapeQueueImpl) processSingleKapiEvent(event *kapiEvent) {
	log := q.log.WithValues("op", "onKapiUpdated", "namespace", event.Namespace, "pod", event.PodName)

	q.targetLock.Lock()
	defer q.targetLock.Unlock()

	switch event.EventType {
	case input_data_registry.KapiEventCreate:
		q.targets.PushFront(&scrapeTarget{Namespace: event.Namespace, PodName: event.PodName})
		log.V(app.VerbosityVerbose).Info("Target added")
	case input_data_registry.KapiEventDelete:
		for listElement := q.targets.Front(); listElement != nil; listElement = listElement.Next() {
			target := listElement.Value.(*scrapeTarget)
			if target.Namespace == event.Namespace && target.PodName == event.PodName {
				q.targets.Remove(listElement)
				break
			}
		}
	}

	targetCount := q.targets.Len()
	rate := float64(targetCount) / q.scrapePeriod.Seconds()
	log.V(app.VerbosityVerbose).Info("New target count", "count", targetCount, "rate", rate)
	// Aim for even temporal distribution of scrapes. Do not track more than targetCount delayed scrapes. targetCount+1
	// would track a second delayed scrape for a target for which we already created rate debt, so don't do that.
	q.pacemaker.UpdateRate(rate, targetCount)
}

//#region Test isolation

// scrapeQueueTestIsolation contains all points of indirection necessary to isolate static function calls
// in the scrapeQueueImpl unit during tests
type scrapeQueueTestIsolation struct {
	// Points to [time.Now]
	TimeNow func() time.Time
}

//#endregion Test isolation

//#region ScrapeQueueFactory

// newScrapeQueueFactory creates a scrapeQueueFactory, configured for productive use
func newScrapeQueueFactory() *scrapeQueueFactory {
	return &scrapeQueueFactory{
		newPacemaker: func(config *pacemakerConfig) pacemaker {
			return newPacemaker(config)
		},
	}
}

// scrapeQueueFactory serves as context for the NewScrapeQueue operation, allowing its dependencies to be replaced
// during test.
type scrapeQueueFactory struct {
	newPacemaker func(config *pacemakerConfig) pacemaker
}

// NewScrapeQueue creates a new scrapeQueueImpl which suggests scraping schedule for the specified
// [input_data_registry.InputDataRegistry].
func (sqf *scrapeQueueFactory) NewScrapeQueue(
	registry input_data_registry.InputDataRegistry, scrapePeriod time.Duration, log logr.Logger) *scrapeQueueImpl {

	queue := &scrapeQueueImpl{
		registry:     registry,
		targets:      list.New(),
		scrapePeriod: scrapePeriod,
		log:          log,
		pacemaker: sqf.newPacemaker(&pacemakerConfig{
			MaxRate:          100,
			RateSurplusLimit: 50,
		}),

		// This channel serves as an update notification buffer, critical to temporally decoupling notification emission,
		// from notification handling. A deadlock occurs if sending blocks. Keep the size of the channel large.
		//
		// Details:
		// While sending a synchronous update notification, the InputDataRegistry is holding a data lock. The same lock
		// must also be acquired by us, as part of data access during notification processing. So this here channel is
		// the implicit second link of a deadlock chain (note that our notification handling consists of a synchronous
		// phase which simply queues the notification on the channel, and an asynchronous phase, which dequeues from
		// channel and does the actual work):
		// 1) InputDataRegistry holds the explicit lock while sending synchronous notifications
		// 2) Our asynchronous phase handler blocks trying to acquire same lock
		// 3) InputDataRegistry synchronously calls our (synchronous phase) handler. It tries to send on the channel. It blocks.
		// 4) Our async phase handler is now waiting for access to registry data. The data registry has locked its data
		// and is waiting to send on our channel. Deadlock!
		//
		// This is solved by two principles:
		// 1) Notification processing is much faster than notification creation.
		// 2) Sending notifications is decoupled from processing them, via a large buffer (the channel).
		updateQueue: make(chan *kapiEvent, 10000),

		testIsolation: scrapeQueueTestIsolation{TimeNow: time.Now},
	}

	// We store the closure in the kapiWatcher field so that we have a fixed memory address for it. We need to pass
	// the same address when unsubscribing.
	queue.kapiWatcher = func(kapi input_data_registry.ShootKapi, event input_data_registry.KapiEventType) {
		queue.onKapiUpdated(kapi, event)
	}
	registry.AddKapiWatcher(&queue.kapiWatcher, true)
	func() {
		queue.targetLock.Lock()
		defer queue.targetLock.Unlock()
		queue.log.V(app.VerbosityVerbose).Info("Initial target count", "count", queue.targets.Len())
	}()

	go queue.processKapiEvents()

	return queue
}

//#endregion ScrapeQueueFactory
