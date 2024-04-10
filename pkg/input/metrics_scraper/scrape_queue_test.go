// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics_scraper

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

//#region Fakes

type FakePacemaker struct {
	MinRate            atomic.Float64
	MaxRate            atomic.Float64
	RateDebtLimit      atomic.Int32
	RateSurplusLimit   atomic.Int32
	PermissionResponse *bool // True = give permission. False = deny. Nil = permit only eager scrapes.
}

func (fp *FakePacemaker) GetScrapePermission(isEagerToScrape bool) bool {
	if fp.PermissionResponse != nil {
		return *fp.PermissionResponse
	}
	return isEagerToScrape
}

func (fp *FakePacemaker) UpdateRate(minRate float64, rateDebtLimit int) {
	fp.MinRate.Store(minRate)
	fp.RateDebtLimit.Store(int32(rateDebtLimit))
}

type FakeShootKapi struct {
	Namespace string
	Name      string
}

func (fsk *FakeShootKapi) ShootNamespace() string {
	return fsk.Namespace
}

func (fsk *FakeShootKapi) PodName() string {
	return fsk.Name
}

func (fsk *FakeShootKapi) PodLabels() map[string]string {
	panic("implement me")
}

func (fsk *FakeShootKapi) TotalRequestCountNew() int64 {
	panic("implement me")
}

func (fsk *FakeShootKapi) TotalRequestCountOld() int64 {
	panic("implement me")
}

func (fsk *FakeShootKapi) MetricsTimeNew() time.Time {
	panic("implement me")
}

func (fsk *FakeShootKapi) MetricsTimeOld() time.Time {
	panic("implement me")
}

func (fsk *FakeShootKapi) PodUID() types.UID {
	panic("implement me")
}

//#endregion Fakes

var _ = Describe("input.metrics_scraper.scrapeQueueImpl", func() {
	const (
		maxRate      = float64(100)
		surplusLimit = 50
		nsName       = "MyNs"
		podName      = "MyPod"
	)

	var (
		newTestScrapeQueue = func(scrapePeriod time.Duration) (*scrapeQueueImpl, *input_data_registry.FakeInputDataRegistry, *FakePacemaker) {
			var pm *FakePacemaker
			factory := newScrapeQueueFactory()
			factory.newPacemaker = func(config *pacemakerConfig) pacemaker {
				pm = &FakePacemaker{}
				pm.MinRate.Store(config.MinRate)
				pm.MaxRate.Store(config.MaxRate)
				pm.RateDebtLimit.Store(int32(config.RateDebtLimit))
				pm.RateSurplusLimit.Store(int32(config.RateSurplusLimit))
				pm.PermissionResponse = ptr.To(true)
				return pm
			}
			idr := &input_data_registry.FakeInputDataRegistry{}
			return factory.NewScrapeQueue(idr, scrapePeriod, logr.Discard()), idr, pm
		}

		// Executes an arbitrary number of GetNext(), then adds the specified target, then does one last GetNext()
		addTargetScrambleQueue = func(nsName, podName string, sq *scrapeQueueImpl, idr input_data_registry.InputDataRegistry) {
			idr.SetKapiData(nsName, podName, "", nil, "")
			sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName}, input_data_registry.KapiEventCreate)
			Eventually(func() bool {
				next := sq.GetNext()
				return next != nil && next.PodName == podName
			}).Should(BeTrue())
		}
	)

	Describe("ScrapeQueueFactory.NewScrapeQueue", func() {
		It("should configure the pacemaker with MinRate = 0, MaxRate = 100, DebtLimit = 0 SurplusLimit = 50", func() {
			// Arrange

			// Act
			sq, _, pm := newTestScrapeQueue(1 * time.Minute)
			defer sq.Close()

			// Assert
			Expect(pm.MinRate.Load()).To(BeZero())
			Expect(pm.MaxRate.Load()).To(Equal(maxRate))
			Expect(pm.RateDebtLimit.Load()).To(BeZero())
			Expect(int(pm.RateSurplusLimit.Load())).To(Equal(surplusLimit))
		})

		It("should subscribe the scrapeQueue for InputDataRegistry events, including events for objects already in the registry", func() {
			// Arrange

			// Act
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			defer sq.Close()

			// Assert
			Expect(idr.Watcher).To(Not(BeNil()))
			Expect(idr.ShouldWatcherNotifyOfPreexisting).To(BeTrue())
		})
	})

	Describe("onKapiUpdated", func() {
		Context("if the event is an add", func() {
			It("should update the pacemaker with MinRate = ScrapeTargetCount / ScrapePeriod, DebtLimit = 0", func() {
				// Arrange
				sq, _, pacemaker := newTestScrapeQueue(1 * time.Minute)
				defer sq.Close()
				kapiCount := 10

				// Act
				for i := 0; i < kapiCount; i++ {
					sq.onKapiUpdated(&FakeShootKapi{Namespace: "my-ns", Name: fmt.Sprintf("my-pod%d", i)}, input_data_registry.KapiEventCreate)
				}

				// Assert
				Eventually(func() bool {
					return pacemaker.MinRate.Load() == float64(kapiCount)/60 &&
						pacemaker.MaxRate.Load() == maxRate &&
						int(pacemaker.RateDebtLimit.Load()) == kapiCount &&
						pacemaker.RateSurplusLimit.Load() == surplusLimit
				}).Should(BeTrue())
			})

			It("should add the new target to the queue", func() {
				// Arrange
				sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
				defer sq.Close()
				idr.SetKapiData(nsName, podName, "", nil, "")

				// Act
				sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName}, input_data_registry.KapiEventCreate)

				// Assert
				Eventually(func() bool {
					next := sq.GetNext()
					if next == nil {
						return false
					}

					return next.PodName == podName && next.Namespace == nsName
				}).Should(BeTrue())
			})
		})

		Context("if the event is a remove", func() {
			It("should update the pacemaker with MinRate = ScrapeTargetCount / ScrapePeriod, DebtLimit = 0", func() {
				// Arrange
				sq, _, pacemaker := newTestScrapeQueue(1 * time.Minute)
				defer sq.Close()
				kapiCount := 10

				// Act
				for i := 0; i < 2*kapiCount; i++ {
					sq.onKapiUpdated(&FakeShootKapi{Namespace: "my-ns", Name: fmt.Sprintf("my-pod%d", i)}, input_data_registry.KapiEventCreate)
				}
				for i := 0; i < kapiCount; i++ {
					sq.onKapiUpdated(&FakeShootKapi{Namespace: "my-ns", Name: fmt.Sprintf("my-pod%d", i)}, input_data_registry.KapiEventDelete)
				}

				// Assert
				Eventually(func() bool {
					return pacemaker.MinRate.Load() == float64(kapiCount)/60 &&
						pacemaker.MaxRate.Load() == maxRate &&
						int(pacemaker.RateDebtLimit.Load()) == kapiCount &&
						pacemaker.RateSurplusLimit.Load() == surplusLimit
				}).Should(BeTrue())
			})

			It("should remove the new target from the queue", func() {
				// Arrange
				sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
				sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
				defer sq.Close()
				addTargetScrambleQueue(nsName, podName, sq, idr)
				addTargetScrambleQueue(nsName, podName+"2", sq, idr)
				sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)

				// Act
				sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName}, input_data_registry.KapiEventDelete)

				// Assert
				Eventually(func() bool {
					// Two consecutive GetNext() calls should return the same Kapi, meaning it's alone in the queue
					next := sq.GetNext()
					if next == nil || next.PodName != podName+"2" {
						return false
					}
					next = sq.GetNext()
					return next != nil && next.PodName == podName+"2"
				}).Should(BeTrue())
			})

			It("should have no effect if the target is missing", func() {
				// Arrange
				sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
				defer sq.Close()
				addTargetScrambleQueue(nsName, podName, sq, idr)
				// Add the second Kapi to the registry, but not to the queue
				idr.SetKapiData(nsName, podName+"2", "", nil, "")
				sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)

				// Act
				sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName + "2"}, input_data_registry.KapiEventDelete)

				// Assert
				Consistently(func() bool {
					next := sq.GetNext()
					return next != nil && next.PodName == podName
				}).Should(BeTrue())
			})
		})

		Context("if the event is of unknown type", func() {
			It("should have no effect", func() {
				// Arrange
				sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
				defer sq.Close()
				idr.SetKapiData(nsName, podName, "", nil, "")
				sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName}, input_data_registry.KapiEventCreate)
				Eventually(func() bool {
					next := sq.GetNext()
					return next != nil && next.PodName == podName
				}).Should(BeTrue())
				sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)

				// Act
				sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName + "2"}, 0xBADF00D)

				// Assert
				Consistently(func() bool {
					next := sq.GetNext()
					return next != nil && next.PodName == podName
				}).Should(BeTrue())
			})
		})
	})

	Describe("GetNext", func() {
		It("should return nil if the queue contains only targets which are missing from the registry", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			idr.RemoveKapiData(nsName, podName)

			// Act
			result := sq.GetNext()

			// Assert
			Expect(result).To(BeNil())
		})

		It("on a queue with multiple targets and a newly added target, should immediately request an eager scrape for the new target", func() {
			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			idr.SetKapiData(nsName, podName+"2", "", nil, "")
			sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName + "2"}, input_data_registry.KapiEventCreate)
			Eventually(sq.Count).Should(Equal(2))
			pm.PermissionResponse = nil // Only allow eager scrapes

			// Act
			next := sq.GetNext()

			// Assert
			Expect(next.PodName).To(Equal(podName + "2"))
		})

		It("should request a scrape operation from the scrape client, if the pacemaker grants permission", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)

			// Act
			next := sq.GetNext()

			// Assert
			Expect(next).To(Not(BeNil()))
		})

		It("should not request a scrape operation from the scrape client, if the pacemaker denies permission", func() {
			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			pm.PermissionResponse = ptr.To(false)

			// Act
			next := sq.GetNext()

			// Assert
			Expect(next).To(BeNil())
		})

		It("upon successful scrape, should record the scrape time in the data registry", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)

			// Act
			sq.GetNext()

			// Assert
			Expect(idr.GetKapiData(nsName, podName).LastMetricsScrapeTime).To(Equal(testutil.NewTimeNowStub(2, 0, 0)()))
		})

		It("should not change the last scrape time for the Kapi, if the pacemaker denies permission", func() {
			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			initialScrapeTime := idr.GetKapiData(nsName, podName).LastMetricsScrapeTime
			pm.PermissionResponse = ptr.To(false)

			// Act
			sq.GetNext()

			// Assert
			Expect(idr.GetKapiData(nsName, podName).LastMetricsScrapeTime).To(Equal(initialScrapeTime))
		})

		It("should return targets in a strictly cyclic order", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()

			// Arrange - add three targets
			for i := 2; i >= 0; i-- {
				addTargetScrambleQueue(nsName, getIndexedPodName(i), sq, idr)
			}

			// Arrange - record the target order
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			var namesInOrder [3]string
			for i := 0; i < 3; i++ {
				namesInOrder[i] = sq.GetNext().PodName
			}

			// Act and assert
			for iteration := 0; iteration < 5; iteration++ {
				sq.testIsolation.TimeNow = testutil.NewTimeNowStub(3+iteration, 0, 0)
				for i := 0; i < 3; i++ {
					next := sq.GetNext()
					Expect(next.PodName).To(Equal(namesInOrder[i]))
				}
			}
		})

		It("should return nil if there are only ineligible targets at the time of the call", func() {
			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			addTargetScrambleQueue(nsName, podName, sq, idr)
			addTargetScrambleQueue(nsName, podName+"2", sq, idr)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			pm.PermissionResponse = nil
			Expect(sq.GetNext()).To(Not(BeNil())) // These two are eager scrapes
			Expect(sq.GetNext()).To(Not(BeNil()))

			// Act
			next := sq.GetNext() // This one is not eager

			// Assert
			Expect(next).To(BeNil())
		})

		It("should return nil, if the queue is empty", func() {
			// Arrange
			sq, _, _ := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()

			// Act
			next := sq.GetNext()

			// Assert
			Expect(next).To(BeNil())
		})

		It("should request an eager/lazy scrape from the pacemaker, depending on whether one scrape period has "+
			"elapsed since the time the next target was last scraped", func() {

			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()

			for i := 0; i < 2; i++ {
				addTargetScrambleQueue(nsName, getIndexedPodName(i), sq, idr)
			}

			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			pm.PermissionResponse = nil
			Expect(sq.GetNext()).NotTo(BeNil()) // These two are eager scrapes
			Expect(sq.GetNext()).NotTo(BeNil())
			Expect(sq.GetNext()).To(BeNil()) // Not eager
			firstTimeTargetsBecomeEligible := sq.testIsolation.TimeNow().Add(sq.scrapePeriod)
			sq.testIsolation.TimeNow = func() time.Time {
				return firstTimeTargetsBecomeEligible
			}

			// Act and assert
			Expect(sq.GetNext()).NotTo(BeNil()) // Eager again
			Expect(sq.GetNext()).NotTo(BeNil()) // Eager again
			Expect(sq.GetNext()).To(BeNil())    // Not eager
		})

		It("should skip targets which are missing from the registry, and return the first target which is not missing", func() {
			// Arrange
			sq, idr, pm := newTestScrapeQueue(1 * time.Minute)
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			defer sq.Close()
			for i := 0; i < 10; i++ {
				addTargetScrambleQueue(nsName, getIndexedPodName(i), sq, idr)
			}
			for i := 0; i < 5; i++ {
				idr.RemoveKapiData(nsName, getIndexedPodName(i))
			}
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			pm.PermissionResponse = nil

			// Act and assert
			for i := 0; i < 5; i++ {
				Expect(sq.GetNext()).ToNot(BeNil()) // One eager scrape for each of the remaining targets
			}
			Expect(sq.GetNext()).To(BeNil()) // This should be back to the first target, thus not eager
		})
	})

	Describe("DueCount", func() {
		It("on an empty queue should return zero", func() {
			// Arrange
			sq, _, _ := newTestScrapeQueue(1 * time.Minute)

			// Act
			due := sq.DueCount(time.Now(), false)

			// Assert
			Expect(due).To(BeZero())
		})

		It("should return zero if the queue contains only targets which are missing from the registry", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			defer sq.Close()
			sq.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			addTargetScrambleQueue(nsName, podName, sq, idr)
			idr.RemoveKapiData(nsName, podName)

			// Act
			due := sq.DueCount(testutil.NewTimeNowStub(2, 0, 0)(), false)

			// Assert
			Expect(due).To(BeZero())
		})

		It("should count targets exactly after one scraping period passes from their last scrape. It should count "+
			"targets which have never been scraped, if, and only if the excludeUnscraped parameter is false", func() {

			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			for i := 0; i < 30; i++ {
				addTargetScrambleQueue(nsName, getIndexedPodName(i), sq, idr)
			}

			firstScrapeTime := testutil.NewTimeNowStub(1, 0, 0)()
			secondScrapeTime := firstScrapeTime.Add(sq.scrapePeriod)
			thirdScrapeTime := secondScrapeTime.Add(sq.scrapePeriod)
			for i := 0; i < 30; i++ {
				next := sq.GetNext()
				if i < 10 {
					idr.SetKapiLastScrapeTime(next.Namespace, next.PodName, time.Time{})
				} else if i < 20 {
					idr.SetKapiLastScrapeTime(next.Namespace, next.PodName, firstScrapeTime)
				} else {
					idr.SetKapiLastScrapeTime(next.Namespace, next.PodName, secondScrapeTime)
				}
			}

			// Act and assert
			Expect(sq.DueCount(secondScrapeTime.Add(-time.Millisecond), false)).To(Equal(10))
			Expect(sq.DueCount(secondScrapeTime.Add(-time.Millisecond), true)).To(Equal(0))
			Expect(sq.DueCount(secondScrapeTime, false)).To(Equal(20))
			Expect(sq.DueCount(secondScrapeTime, true)).To(Equal(10))
			Expect(sq.DueCount(thirdScrapeTime, false)).To(Equal(30))
			Expect(sq.DueCount(thirdScrapeTime, true)).To(Equal(20))
		})
	})

	Describe("Close", func() {
		It("should terminate the scrapeQueue's subscription to InputDataRegistry events", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)
			Expect(idr.Watcher).NotTo(BeNil())

			// Act
			sq.Close()

			// Assert
			Expect(idr.Watcher).To(BeNil())
		})

		It("should terminate the processing of InputDataRegistry events", func() {
			// Arrange
			sq, idr, _ := newTestScrapeQueue(1 * time.Minute)

			// Act
			sq.Close()

			// Assert
			idr.SetKapiData(nsName, podName, "", nil, "")
			sq.onKapiUpdated(&FakeShootKapi{Namespace: nsName, Name: podName}, input_data_registry.KapiEventCreate)
			Consistently(sq.GetNext).Should(BeNil())
		})
	})
})
