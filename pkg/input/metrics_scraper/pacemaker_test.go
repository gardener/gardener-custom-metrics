package metrics_scraper

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.metrics_scraper.pacemakerImpl", func() {
	var (
		newTestPacemaker = func(minRate, maxRate float64, rateDebtLimit, rateSurplusLimit int) *pacemakerImpl {
			return newPacemaker(&pacemakerConfig{
				MinRate:          minRate,
				MaxRate:          maxRate,
				RateDebtLimit:    rateDebtLimit,
				RateSurplusLimit: rateSurplusLimit,
			})
		}

		// newTestPacemakerWithTestWorthyConfiguration creates a pacemaker with a configuration which engages all it its
		// features. Rates and limits are not-too-large-not-too-small, e.g. the surplus limit is greater than 1.
		// The values are also suitable for an actual test run, e.g. the debt limit small enough, so executing that many
		// calls in test, would take negligible time
		newTestPacemakerWithTestWorthyConfiguration = func() *pacemakerImpl {
			return newPacemaker(&pacemakerConfig{
				MinRate:          2,
				MaxRate:          4,
				RateDebtLimit:    20,
				RateSurplusLimit: 10,
			})
		}
	)

	Describe("newPacemaker", func() {
		It("should create a pacemaker with the specified configuration values", func() {
			// Arrange
			creationConfig := &pacemakerConfig{
				MinRate:          10,
				MaxRate:          20,
				RateDebtLimit:    200,
				RateSurplusLimit: 100,
			}

			// Act
			pm := newPacemaker(creationConfig)

			// Assert
			Expect(pm.config.MinRate).To(Equal(creationConfig.MinRate))
			Expect(pm.config.MaxRate).To(Equal(creationConfig.MaxRate))
			Expect(pm.config.RateDebtLimit).To(Equal(creationConfig.RateDebtLimit))
			Expect(pm.config.RateSurplusLimit).To(Equal(creationConfig.RateSurplusLimit))
		})
		It("should create a pacemaker with zero debt", func() {
			// Arrange
			creationConfig := &pacemakerConfig{
				MinRate:          10,
				MaxRate:          20,
				RateDebtLimit:    200,
				RateSurplusLimit: 100,
			}

			// Act
			pm := newPacemaker(creationConfig)

			// Assert
			Expect(pm.GetScrapePermission(false)).To(BeFalse())
		})
		It("should create a pacemaker with zero surplus", func() {
			// Arrange
			creationConfig := &pacemakerConfig{
				MinRate:          10,
				MaxRate:          20,
				RateDebtLimit:    2,
				RateSurplusLimit: 1,
			}

			// Act and assert
			pm := newPacemaker(creationConfig)
			Expect(pm.GetScrapePermission(true)).To(BeTrue())
			Expect(pm.GetScrapePermission(true)).To(BeFalse())
		})
	})
	Describe("UpdateRate", func() {
		It("should write the specified MinRate value to the pacemaker's configuration", func() {
			// Arrange
			pm := newTestPacemakerWithTestWorthyConfiguration()

			// Act
			pm.UpdateRate(777, 1000)

			// Assert
			Expect(pm.config.MinRate).To(Equal(float64(777)))
		})
		It("should write the specified RateDebtLimit value to the pacemaker's configuration", func() {
			// Arrange
			pm := newTestPacemakerWithTestWorthyConfiguration()

			// Act
			pm.UpdateRate(10, 777)

			// Assert
			Expect(pm.config.RateDebtLimit).To(Equal(777))
		})
	})
	Describe("GetScrapePermission", func() {
		Context("if the scrape is eager", func() {
			Context("starting from a state of zero debt and surplus", func() {
				It("should allow RateSurplusLimit immediate calls, and deny the next immediate call", func() {
					// Arrange
					rateSurplusLimit := 10
					pm := newTestPacemaker(2, 4, 20, rateSurplusLimit)

					// Act and assert
					for i := 0; i < rateSurplusLimit; i++ {
						Expect(pm.GetScrapePermission(true)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(true)).To(BeFalse())
				})
			})
			Context("starting from a state of exhausted surplus", func() {
				It("should grant permission to as many calls, as correspond to MaxRate, and deny the next one", func() {
					// Arrange
					maxRate := 4.0
					secondsElapsed := 5
					expectedAllowedCalls := int(maxRate * float64(secondsElapsed))
					rateSurplusLimit := expectedAllowedCalls + 5

					pm := newTestPacemaker(2, maxRate, 10, rateSurplusLimit)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Exhaust the surplus
					for i := 0; i < rateSurplusLimit; i++ {
						Expect(pm.GetScrapePermission(true)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(true)).To(BeFalse())

					// Now advance time. All subsequent allowance should be due to rate, not surplus
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, secondsElapsed)

					// Act and assert
					for i := 0; i < expectedAllowedCalls; i++ {
						Expect(pm.GetScrapePermission(true)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(true)).To(BeFalse())
				})
			})
			Context("starting from a state of high debt", func() {
				It("should allow RateSurplusLimit immediate calls, then deny the next call", func() {

					// Arrange
					surplusLimit := 10
					pm := newTestPacemaker(5, 10, 50, surplusLimit)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Start the timer
					Expect(pm.GetScrapePermission(true)).To(BeTrue())
					Expect(pm.GetScrapePermission(false)).To(BeFalse())

					// Advance time to accumulate debt
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 0)

					// Act and assert
					for i := 0; i < surplusLimit; i++ {
						Expect(pm.GetScrapePermission(true)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(false)).To(BeFalse())
				})
			})
		})
		Context("if the scrape is not eager", func() {
			Context("starting from a state of zero debt", func() {
				It("after a period of inactivity which does not exceed the debt limit, should allow as many "+
					"immediate calls, as correspond to MinRate, and deny the next one", func() {

					// Arrange
					minRate := 1.5
					secondsElapsed := 4
					expectedAllowedCalls := int(minRate * float64(secondsElapsed))

					pm := newTestPacemaker(minRate, 100, 100, 100)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Start the timer
					Expect(pm.GetScrapePermission(true)).To(BeTrue())
					Expect(pm.GetScrapePermission(false)).To(BeFalse())

					// Now advance time. All subsequent allowance should be due to accumulated debt
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, secondsElapsed)

					// Act and assert
					for i := 0; i < expectedAllowedCalls; i++ {
						Expect(pm.GetScrapePermission(false)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(false)).To(BeFalse())
				})
				It("after a period of inactivity which exceeds the debt limit, should allow RateDebtLimit "+
					"immediate calls, and deny the next one", func() {

					// Arrange
					minRate := 1.5
					secondsElapsed := 40
					debtLimit := 5

					pm := newTestPacemaker(minRate, 100, debtLimit, 100)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Start the timer
					Expect(pm.GetScrapePermission(true)).To(BeTrue())
					Expect(pm.GetScrapePermission(false)).To(BeFalse())

					// Now advance time. All subsequent allowance should be due to accumulated debt
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, secondsElapsed)

					// Act and assert
					for i := 0; i < debtLimit; i++ {
						Expect(pm.GetScrapePermission(false)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(false)).To(BeFalse())
				})
				It("if time has not passed, should not allow any immediate calls", func() {
					// Arrange
					pm := newTestPacemaker(2, 4, 20, 10)

					// Act and assert
					isAllowed := pm.GetScrapePermission(false)

					// Assert
					Expect(isAllowed).To(BeFalse())
				})
			})
		})
		Context("starting from a state of high debt", func() {
			It("should allow as many immediate calls, as the value of the initial debt, if they don't exceed "+
				"MaxRate, and then deny the next call if it is not eager", func() {

				// Arrange
				secondsElapsed := 2
				minRate := 2.0
				expectedAllowedCalls := int(minRate * float64(secondsElapsed))

				for _, isEager := range []bool{true, false} {
					pm := newTestPacemaker(minRate, 10, 1000, 1000)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Start the timer
					Expect(pm.GetScrapePermission(true)).To(BeTrue())
					Expect(pm.GetScrapePermission(false)).To(BeFalse())

					// Advance time to accumulate debt
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, secondsElapsed)

					// Act and assert
					for i := 0; i < expectedAllowedCalls; i++ {
						Expect(pm.GetScrapePermission(isEager)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(false)).To(BeFalse())
				}
			})
			It("should still be limited to MaxRate", func() {
				// Arrange
				secondsElapsed := 2
				surplusLimit := 30 // Must be smaller than debt limit, so it can be exhausted while still in debt
				maxRate := 10.0
				expectedAllowedCalls := int(maxRate * float64(secondsElapsed))

				for _, isEager := range []bool{true, false} {
					pm := newTestPacemaker(5, maxRate, 1000, surplusLimit)
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

					// Start the timer
					Expect(pm.GetScrapePermission(true)).To(BeTrue())
					Expect(pm.GetScrapePermission(false)).To(BeFalse())

					// Advance time to accumulate debt
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 0)

					// Consume the surplus
					for i := 0; i < surplusLimit; i++ {
						Expect(pm.GetScrapePermission(true)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(true)).To(BeFalse())

					// Advance time a bit more to accumulate allowance based on MaxRate
					pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, secondsElapsed)

					// Act and assert
					for i := 0; i < expectedAllowedCalls; i++ {
						Expect(pm.GetScrapePermission(isEager)).To(BeTrue())
					}
					Expect(pm.GetScrapePermission(isEager)).To(BeFalse())
				}
			})
		})
		It("should perform as expected in one complex scenario", func() {
			// This one last test case does not follow the good practice of simplicity and testing just one thing.
			// It uses a complex scenario, in attempt to catch potential issues missed by the above simple cases.

			debtLimit := 30
			surplusLimit := 20 // Must be smaller than debt limit for code below to work
			minRate := 1.0
			maxRate := 10.0
			pm := newTestPacemaker(minRate, maxRate, debtLimit, surplusLimit)
			pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

			// Start the timer
			Expect(pm.GetScrapePermission(true)).To(BeTrue())
			Expect(pm.GetScrapePermission(false)).To(BeFalse())

			// Stay idle until debt limit exceeded
			pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 0) // Debt=30, SurplusAllowance=20
			Expect(pm.GetScrapePermission(false)).To(BeTrue())
			Expect(pm.GetScrapePermission(true)).To(BeTrue())

			// Mixed eager+lazy calls until surplus is exhausted
			for i := 0; i < surplusLimit-2-1; i++ { // -2 for the immediately preceding 2 calls, -1 for the call below
				Expect(pm.GetScrapePermission(i%2 == 0)).To(BeTrue())
			}
			Expect(pm.GetScrapePermission(false)).To(BeTrue()) // Last available surplus allowance
			Expect(pm.GetScrapePermission(true)).To(BeFalse()) // Hits surplus limit

			// Wait a bit to recover surplus allowance
			// Debt=10, SurplusAllowance=0
			pm.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 2) // Debt=12, SurplusAllowance=20

			// Fulfill debt by a mix of eager and lazy calls
			for i := 0; i < 12-1; i++ { // -1 for the call below
				Expect(pm.GetScrapePermission(i%2 == 0)).To(BeTrue())
			}
			Expect(pm.GetScrapePermission(false)).To(BeTrue())  // Covers last debt
			Expect(pm.GetScrapePermission(false)).To(BeFalse()) // Hits zero debt
			Expect(pm.GetScrapePermission(true)).To(BeTrue())   // Surplus available

			// Stay idle until debt limit exceeded again
			pm.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0) // Debt=30, SurplusAllowance=20
			Expect(pm.GetScrapePermission(false)).To(BeTrue())
			Expect(pm.GetScrapePermission(true)).To(BeTrue())
		})
	})
})
