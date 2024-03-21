package metrics_scraper

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.metrics_scraper.Scraper", func() {
	const (
		nsName       = "MyNs"
		scrapePeriod = 1 * time.Minute
	)

	var (
		// Creates a test scraper instance which works well as starting point for most tests. The queue is empty.
		newTestScraper = func() (
			*Scraper,
			*input_data_registry.FakeInputDataRegistry,
			*fakeScrapeQueue,
			*fakeMetricsClient,
			*fakeTicker,
			*scraperTestMetrics) {

			clientMetrics := &scraperTestMetrics{}
			schedulingPeriod := 50 * time.Millisecond
			idr := &input_data_registry.FakeInputDataRegistry{}
			fakeQueue := newFakeScrapeQueue(idr, scrapePeriod)
			fakeTicker := newFakeTicker()
			fakeClient := &fakeMetricsClient{}

			scraper := NewScraper(idr, scrapePeriod, schedulingPeriod, logr.Discard())
			scraper.queue = fakeQueue
			scraper.testIsolation.NewTicker = func(period time.Duration) ticker {
				fakeTicker.Period.Store(int64(period))
				return fakeTicker
			}
			scraper.testIsolation.NewMetricsClient = func() metricsClient {
				return fakeClient
			}
			scraper.testIsolation.workerProc = func(ctx context.Context) {
				clientMetrics.WorkerProcCount.Add(1)
				scraper.workerWaitGroup.Done()
				scraper.activeWorkerCount.Add(-1)
			}

			return scraper, idr, fakeQueue, fakeClient, fakeTicker, clientMetrics
		}
		// Applied to scraper created by newTestScraper. Populates it with Kapis and targets, and puts it in a state as
		// if it has already been scraping. Some of the parameters control the effects of the forged last scrape shift.
		setScraperState = func(
			scraper *Scraper,
			idr *input_data_registry.FakeInputDataRegistry,
			sq *fakeScrapeQueue,
			lastShiftTime time.Time,
			lastShiftTargetCount int,
			lastShiftWorkerCount int,
			leftoverCount int,
			thisShiftTargetTotalCount int) {

			scraper.lastShiftStartTime = lastShiftTime
			scraper.lastShiftScrapeTargetCount = lastShiftTargetCount
			scraper.lastShiftWorkerCount = lastShiftWorkerCount
			for i := 0; i < thisShiftTargetTotalCount; i++ {
				sq.Queue = append(sq.Queue, &scrapeTarget{nsName, getIndexedPodName(i)})
				idr.SetKapiData(nsName, getIndexedPodName(i), "", nil, "")
				if i < thisShiftTargetTotalCount-lastShiftTargetCount {
					// Newly added since last shift. Leave scrape time unset.
				} else if i < thisShiftTargetTotalCount-lastShiftTargetCount+leftoverCount {
					// Leftover from last shift. Use some non-zero scrape time before last shift.
					idr.SetKapiLastScrapeTime(nsName, getIndexedPodName(i), testutil.NewTime(1, 0, 0))
				} else {
					// Successfully scraped during last shift
					idr.SetKapiLastScrapeTime(nsName, getIndexedPodName(i), lastShiftTime)
				}
			}
			scraper.testIsolation.TimeNow = func() time.Time { return lastShiftTime }
		}
		// Prepares a bunch of objects which work well as a starting point for most test of the scrape worker procedure
		// The queue has only one target, and GetNext() permanently dequeues the target, so it can be scraped only once.
		arrangeWorkerTest = func() (
			*Scraper,
			*input_data_registry.FakeInputDataRegistry,
			*fakeMetricsClient,
			*scraperTestMetrics,
			*scrapeTarget) {

			scraper, idr, sq, client, _, testMetrics := newTestScraper()
			sq.IsNoRequeue = true
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 1, 1, 1, 1)
			target := sq.Queue[0]
			// Upon exit, workers check out with the wait group and some other counters. Prime those so they'd accept
			// the checkout.
			scraper.workerWaitGroup.Add(1)
			scraper.activeWorkerCount.Add(1)

			return scraper, idr, client, testMetrics, target
		}
	)

	Describe("ScraperFactory.NewScraper", func() {
		It("should configure the scraper queue with the specified scrapePeriod", func() {
			// Arrange
			scrapePeriod := 5 * time.Minute

			// Act
			scraper := NewScraper(
				input_data_registry.NewInputDataRegistry(0, logr.Discard()),
				scrapePeriod,
				100*time.Millisecond,
				logr.Discard())

			// Assert
			Expect(scraper.queue.(*scrapeQueueImpl).scrapePeriod).To(Equal(scrapePeriod))
		})
	})

	Describe("Start", func() {
		It("should poll until context cancelled, and stop polling when the context is cancelled", func() {
			// Arrange
			scraper, _, _, _, ticker, _ := newTestScraper()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var isRunning atomic.Bool
			scraper.testIsolation.workerProc = func(ctx context.Context) {
				isRunning.Store(true)
				scraper.workerWaitGroup.Done()
				scraper.activeWorkerCount.Add(-1)
			}

			// Act and assert
			go func() {
				isRunning.Store(false)
				scraper.Start(ctx)
			}()
			ticker.Channel <- testutil.NewTime(1, 1, 0)
			Eventually(isRunning.Load).Should(BeTrue())
			isRunning.Store(false)
			ticker.Channel <- testutil.NewTime(1, 2, 0)
			Eventually(isRunning.Load).Should(BeTrue())
			cancel()
			isRunning.Store(false)
			abortChan := make(chan bool)
			go func() {
				select {
				case ticker.Channel <- testutil.NewTime(1, 3, 0):
					// The expectation is, because context has been cancelled, no one will be there to read from the channel
					break
				case <-abortChan:
					break
				}
			}()
			Consistently(isRunning.Load).Should(BeFalse())
			abortChan <- true
		})

		It("should not exit before all workers exit", func() {
			// Arrange
			scraper, idr, sq, _, ticker, _ := newTestScraper()
			sq.Queue = append(sq.Queue, &scrapeTarget{nsName, getIndexedPodName(0)})
			idr.SetKapiData(nsName, getIndexedPodName(0), "", nil, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var isRunning atomic.Bool
			scraper.testIsolation.workerProc = func(ctx context.Context) {
				isRunning.Store(true)
			}

			// Act and assert
			go func() {
				scraper.Start(ctx)
				isRunning.Store(false)
			}()
			ticker.Channel <- testutil.NewTime(1, 1, 0)
			Eventually(isRunning.Load).Should(BeTrue())
			Consistently(isRunning.Load).Should(BeTrue())
			// Ensure the first precondition to scraper.Start() exiting - context cancelled. After that it will only be
			// blocked by the worker wait group
			cancel()
			scraper.workerWaitGroup.Done() // Simulate the worker exiting
			scraper.activeWorkerCount.Add(-1)
			Eventually(isRunning.Load).Should(BeFalse())
		})

		It("should close scrape queue before exiting", func() {
			// Arrange
			scraper, idr, sq, _, _, _ := newTestScraper()
			sq.Queue = append(sq.Queue, &scrapeTarget{nsName, getIndexedPodName(0)})
			idr.SetKapiData(nsName, getIndexedPodName(0), "", nil, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var isRunning atomic.Bool
			isRunning.Store(true)

			// Act and assert
			go func() {
				scraper.Start(ctx)
				isRunning.Store(false)
			}()
			cancel()
			Eventually(isRunning.Load).Should(BeFalse())
			Expect(sq.IsClosed()).To(BeTrue())
		})

		It("upon first invocation with multiple targets, should start 2 workers, then, if no scrapes are "+
			"recorded, double upon each ticker tick, until it gets capped to the number of targets", func() {

			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 9, 1, 9, 9)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			for testIteration, expected := range []int32{2, 4, 8, 9} {
				metrics.WorkerProcCount.Store(0)
				now := testutil.NewTimeNowStub(3, 0, testIteration)
				scraper.testIsolation.TimeNow = now
				ticker.Channel <- now()
				// Make sure some workers have started
				Eventually(func() bool { return metrics.WorkerProcCount.Load() > 0 }).Should(BeTrue())
				// Make sure the workers have stopped
				Eventually(scraper.activeWorkerCount.Load).Should(BeZero())
				Eventually(metrics.WorkerProcCount.Load).Should(Equal(expected))
				Consistently(metrics.WorkerProcCount.Load).Should(Equal(expected))
			}
		})

		It("if last shift managed to scrape all of its targets, should attempt scraping with one less worker", func() {
			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			scraper.lastShiftScrapeTargetCount = 10
			scraper.lastShiftWorkerCount = 10
			for i := 0; i < 12; i++ {
				sq.Queue = append(sq.Queue, &scrapeTarget{nsName, getIndexedPodName(i)})
				idr.SetKapiData(nsName, getIndexedPodName(i), "", nil, "")
				idr.SetKapiLastScrapeTime(nsName, getIndexedPodName(i), testutil.NewTime(1, 0, 0))
			}
			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 0)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)
			ticker.Channel <- testutil.NewTime(1, 1, 0)

			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(9)))
		})

		It("if last shift failed to scrape all of its targets, should increase worker count proportionally to "+
			"last shift's throughput and current due target count", func() {

			// Arrange
			// Last shift scraped 10 out of 11 targets with 5 workers. This shift has 12 targets. Expected worker count
			// at estimated worker velocity=2, is 6
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 11, 5, 1, 12)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(3, 0, 0)
			ticker.Channel <- testutil.NewTime(3, 0, 0)
			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(6)))
			Consistently(metrics.WorkerProcCount.Load).Should(Equal(int32(6)))
		})

		It("should consider leftover targets from last shift, when calculating this shift's worker count", func() {
			// Arrange
			// Last shift scraped 1 out of 6 targets with 6 workers. This shift has 3 new targets and 5 leftover
			// targets. Expected worker count at bounded worker velocity=0.133..->1, is 8
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 6, 6, 5, 8)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(3, 0, 0)
			ticker.Channel <- testutil.NewTime(3, 0, 0)
			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(8)))
			Consistently(metrics.WorkerProcCount.Load).Should(Equal(int32(8)))
		})

		It("should respect maxShiftWorkerCount", func() {
			// Arrange
			// Last shift scraped 1 out of 6 targets with 6 workers. This shift has 10 new targets and 5 leftover
			// targets. Expected worker count at bounded worker velocity=0.133..->1, is 15, but should be capped to 10
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 6, 6, 5, 15)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(3, 0, 0)
			ticker.Channel <- testutil.NewTime(3, 0, 0)
			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(10)))
			Consistently(metrics.WorkerProcCount.Load).Should(Equal(int32(10)))
		})

		It("should respect minShiftWorkerCount", func() {
			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 0, 5, 0, 0)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			for i := 0; i < 20; i++ {
				metrics.WorkerProcCount.Store(0)
				now := testutil.NewTimeNowStub(3, 0, i)
				scraper.testIsolation.TimeNow = now
				ticker.Channel <- now()
				// Make sure workers have started
				Eventually(func() bool { return metrics.WorkerProcCount.Load() > 0 }).Should(BeTrue())
				// Make sure the workers have stopped
				Eventually(scraper.activeWorkerCount.Load).Should(Equal(int32(0)))
			}
			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(1)))
			Consistently(metrics.WorkerProcCount.Load).Should(Equal(int32(1)))
		})

		It("if the queue is empty, should slowly reduce the number of workers to 1", func() {
			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 0, 5, 0, 0)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			for testIteration, expected := range []int32{4, 3, 2, 1, 1} {
				metrics.WorkerProcCount.Store(0)
				now := testutil.NewTimeNowStub(3, 0, testIteration)
				scraper.testIsolation.TimeNow = now
				ticker.Channel <- now()
				Eventually(metrics.WorkerProcCount.Load).Should(Equal(expected))
				Consistently(metrics.WorkerProcCount.Load).Should(Equal(expected))
				// Make sure the workers have stopped
				Eventually(scraper.activeWorkerCount.Load).Should(BeZero())
			}
		})

		It("should respect maxActiveWorkerCount", func() {
			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 5, 5, 1, 10)
			scraper.activeWorkerCount.Add(41) // Simulate lots of workers, limit is 50
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			scraper.testIsolation.TimeNow = testutil.NewTimeNowStub(3, 0, 0)
			ticker.Channel <- testutil.NewTime(3, 0, 0)
			// 10 targets at velocity 1 should cause 10 workers. However, because 41 out of a max of 50 workers are
			// counted as active, new workers should be capped to 9
			Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32(9)))
			Consistently(metrics.WorkerProcCount.Load).Should(Equal(int32(9)))
		})

		It("should apply the specified scrapeFlowControlPeriod to the ticker it uses", func() {
			// Arrange
			schedulingPeriod := 100 * time.Millisecond
			fakeTicker := newFakeTicker()
			scraper := NewScraper(
				&input_data_registry.FakeInputDataRegistry{}, time.Minute, schedulingPeriod, logr.Discard())
			scraper.testIsolation.NewTicker = func(period time.Duration) ticker {
				fakeTicker.Period.Store(int64(period))
				return fakeTicker
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.Start(ctx)

			// Assert
			Eventually(fakeTicker.Period.Load).Should(Equal(int64(schedulingPeriod)))
		})

		It("should schedule scrape shifts when and only when the ticket ticks", func() {
			// Arrange
			scraper, idr, sq, _, ticker, metrics := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 5, 5, 1, 5)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act and assert
			go scraper.Start(ctx)

			for i := 0; i < 3; i++ {
				now := testutil.NewTimeNowStub(3, i, 0)
				scraper.testIsolation.TimeNow = now
				ticker.Channel <- now()
				Eventually(metrics.WorkerProcCount.Load).Should(Equal(int32((i + 1) * 5)))
				Eventually(scraper.activeWorkerCount.Load).Should(BeZero())
				Expect(metrics.WorkerProcCount.Load()).To(Equal(int32((i + 1) * 5)))
			}
		})
	})

	Describe("workerProc", func() {
		It("polls the targets returned by GetNext(),until the context is cancelled", func() {
			// Arrange
			scraper, idr, sq, client, _, _ := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 1, 1, 0, 1)
			scraper.workerWaitGroup.Add(1)
			scraper.activeWorkerCount.Add(1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act and assert
			go scraper.workerProc(ctx)
			Eventually(client.WasScraped.Load).Should(BeTrue())
			Consistently(func() bool { return client.WasScraped.Swap(false) }).Should(BeTrue())
			cancel()
			Eventually(func() bool { return client.WasScraped.Swap(false) }).Should(BeFalse())
			Consistently(client.WasScraped.Load).Should(BeFalse())
			Expect(scraper.activeWorkerCount.Load()).To(BeZero())
		})

		It("if context has not been cancelled, polls the queue until GetNext() returns nil", func() {
			// Arrange
			scraper, idr, sq, client, _, _ := newTestScraper()
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 1, 1, 0, 1)
			scraper.workerWaitGroup.Add(1)
			scraper.activeWorkerCount.Add(1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act and assert
			go scraper.workerProc(ctx)
			Eventually(client.WasScraped.Load).Should(BeTrue())
			Consistently(func() bool { return client.WasScraped.Swap(false) }).Should(BeTrue())
			sq.EmptyQueue()
			Eventually(func() bool { return client.WasScraped.Swap(false) }).Should(BeFalse())
			Consistently(client.WasScraped.Load).Should(BeFalse())
			Expect(scraper.activeWorkerCount.Load()).To(BeZero())
		})

		It("if context has been cancelled, exits before scraping the queue", func() {
			// Arrange
			scraper, _, client, _, _ := arrangeWorkerTest()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			// Act
			go scraper.workerProc(ctx)

			// Assert
			Consistently(client.WasScraped.Load).Should(BeFalse())
			Expect(scraper.activeWorkerCount.Load()).To(BeZero())
		})

		It("should scrape each target returned by the queue", func() {
			// Arrange
			scraper, idr, sq, _, _, _ := newTestScraper()
			sq.IsNoRequeue = true
			setScraperState(scraper, idr, sq, testutil.NewTime(2, 0, 0), 5, 5, 0, 5)
			scraper.workerWaitGroup.Add(1)
			scraper.activeWorkerCount.Add(1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			go scraper.workerProc(ctx)

			// Assert
			Eventually(scraper.activeWorkerCount.Load).Should(BeZero())
			for _, kapi := range idr.GetKapis() {
				Expect(kapi.TotalRequestCountNew).To(Equal(fakeMetricsClientMetricsValue))
			}
		})

		Context("when scraping a target", func() {
			It("should have no effect if the kapi is missing from the registry", func() {
				// Arrange
				scraper, idr, client, testMetrics, target := arrangeWorkerTest()
				idr.SetKapis(nil)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Act
				go scraper.workerProc(ctx)

				// Assert
				scraper.workerWaitGroup.Wait()
				Expect(testMetrics.WorkerProcCount.Load()).To(BeZero())
				Expect(client.WasScraped.Load()).To(BeFalse())
				Expect(idr.GetKapiData(target.Namespace, target.PodName)).To(BeNil())
			})

			It("should have no effect if the auth secret is missing from the registry", func() {
				// Arrange
				scraper, idr, client, testMetrics, target := arrangeWorkerTest()
				idr.RemoveShootAuthSecret()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Act
				go scraper.workerProc(ctx)

				// Assert
				scraper.workerWaitGroup.Wait()
				Expect(testMetrics.WorkerProcCount.Load()).To(BeZero())
				Expect(client.WasScraped.Load()).To(BeFalse())
				Expect(idr.GetKapiData(target.Namespace, target.PodName).TotalRequestCountNew).To(BeZero())
				Expect(idr.GetKapiData(target.Namespace, target.PodName).MetricsTimeNew).To(BeZero())
			})

			It("should have no effect if the CA certificate is missing from the registry", func() {
				// Arrange
				scraper, idr, client, testMetrics, target := arrangeWorkerTest()
				idr.HasNoCACertificate = true
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Act
				go scraper.workerProc(ctx)

				// Assert
				scraper.workerWaitGroup.Wait()
				Expect(testMetrics.WorkerProcCount.Load()).To(BeZero())
				Expect(client.WasScraped.Load()).To(BeFalse())
				Expect(idr.GetKapiData(target.Namespace, target.PodName).TotalRequestCountNew).To(BeZero())
				Expect(idr.GetKapiData(target.Namespace, target.PodName).MetricsTimeNew).To(BeZero())
			})

			It("should record the resulting metric value in the registry", func() {
				// Arrange
				scraper, idr, _, _, target := arrangeWorkerTest()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Act
				go scraper.workerProc(ctx)

				// Assert
				Eventually(func() int64 {
					return idr.GetKapiData(target.Namespace, target.PodName).TotalRequestCountNew
				}).Should(Equal(fakeMetricsClientMetricsValue))
			})

			It("should use scrapePeriod / 2 as timeout for individual scrapes", func() {
				// Arrange
				scraper, _, client, _, _ := arrangeWorkerTest()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// Act
				go scraper.workerProc(ctx)

				// Assert
				scraper.workerWaitGroup.Wait()
				actual := client.GetLastContextDuration()
				expected := float64(scrapePeriod) / 2
				relativeDifference := (float64(actual) - expected) / expected
				// Note that this check can fail, if test execution gets sufficiently slowed down. See
				// fakeMetricsClient.GetLastContextDuration.
				// Use generous 10% margin to avoid test flakiness due to sensitivity to timing
				Expect(math.Abs(relativeDifference) < 0.1).To(BeTrue())
				Expect(scraper.scrapeTimeout).To(Equal(scrapePeriod / 2))
			})
		})
	})
})
