package metrics_scraper

import (
	"sync"
	"time"
)

// See newPacemaker.
type pacemakerConfig struct {
	MinRate float64 // Lower rate limit, in scrapes/second. Lazy requests are scheduled at that rate.
	MaxRate float64 // Upper rate limit, in scrapes/second. Eager requests are scheduled at that rate.

	// A client running below MinRate is considered in "rate debt" and will be asked to catch up by temporarily running
	// faster (at MaxRate).
	// This is the limit to the rate debt. If delay greater than this accrues, the excess will not be compensated by
	// running at elevated rate.
	RateDebtLimit int

	// An eager client will be allowed to exceed MaxRate by this many requests. Once the surplus allowance is exhausted,
	// the client is limited to MaxRate. The allowance gradually replenishes when the client is running below MaxRate.
	//
	// The smallest allowed value for the surplus is 1, to allow the first call to GetScrapePermission, which starts the
	// rate-based allowance and debt accumulations
	RateSurplusLimit int
}

// Abstracts the pacemaker implementation available as pacemakerImpl, so it can be replaced for testing purposes.
// See newPacemaker.
type pacemaker interface {
	// GetScrapePermission tells the caller whether to run a scrape operation. The pacemaker assumes that if the function
	// returns true, a scrape operation will be performed by the caller, and counts that scrape.
	// isEagerToScrape:
	// - true - the caller needs to scrape and is asking for permission
	// - false - the caller is just declaring it's able to scrape if pace making requires it
	//
	// The accumulation for allowances and debt starts with the first call to GetScrapePermission
	GetScrapePermission(isEagerToScrape bool) bool
	// UpdateRate updates the [pacemakerConfig.MinRate] and [pacemakerConfig.RateDebtLimit] of the pacemaker.
	UpdateRate(minRate float64, rateDebtLimit int)
}

// Implements the pacemaker interface
// See newPacemaker.
type pacemakerImpl struct {
	config         pacemakerConfig
	lastUpdateTime time.Time
	currentDebt    float64 // How much has rate fallen behind config.MinRate. If >=1, even lazy scrapers scrape
	currentSurplus float64 // How much does rate exceed config.MaxRate. If >config.RateSurplusLimit, even eager scrapers halt
	lock           sync.Mutex

	testIsolation pacemakerTestIsolation // Provides indirections necessary to isolate the unit during tests
}

// newPacemaker creates a rate limiter which maintains the rate of some repeating operation between a set minimum and
// maximum.
// Within the space between min and max, the exact rate is determined depending on whether the client is eager to
// perform the operation. Eager requests are governed by "no more than max rate". Non-eager requests follow a
// "no less than min rate" schedule.
//
// In addition, there are two burst-type parameters, influencing pacemaker's behavior. If the client falls behind the
// min rate, it will be subsequently asked to run at an increased (max) rate until it compensates for the delay. There
// is a limit to how much delay the client will be asked to catch up to, and that is controlled by the
// [pacemakerConfig.RateDebtLimit] field. Similarly, an eager client is allowed to temporarily exceed the max rate,
// but by no more than [pacemakerConfig.RateSurplusLimit].
//
// The accumulation for allowances and debt starts with the first call to GetScrapePermission
func newPacemaker(config *pacemakerConfig) *pacemakerImpl {
	return &pacemakerImpl{
		config: *config,
		testIsolation: pacemakerTestIsolation{
			TimeNow: time.Now,
		},
	}
}

// UpdateRate updates the [pacemakerConfig.MinRate] and [pacemakerConfig.RateDebtLimit] of the pacemaker.
func (p *pacemakerImpl) UpdateRate(minRate float64, rateDebtLimit int) {
	p.lock.Lock()
	p.config.MinRate = minRate
	p.config.RateDebtLimit = rateDebtLimit
	p.lock.Unlock()
}

// GetScrapePermission tells the caller whether to run a scrape operation. The pacemaker assumes that if the function
// returns true, a scrape operation will be performed by the caller, and counts that scrape.
// isEagerToScrape:
// - true - the caller needs to scrape and is asking for permission
// - false - the caller is just declaring it's able to scrape if pace making requires it
//
// The accumulation for allowances and debt starts with the first call to GetScrapePermission
func (p *pacemakerImpl) GetScrapePermission(isEagerToScrape bool) bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	now := p.testIsolation.TimeNow()
	if p.lastUpdateTime.IsZero() {
		p.lastUpdateTime = now
	}
	elapsedSeconds := now.Sub(p.lastUpdateTime).Seconds()
	p.lastUpdateTime = now

	// Reflect the passed time upon debt and surplus.
	// Do not apply bounds until we've also counted the potential scrape we may allow in the current frame.
	p.currentDebt += elapsedSeconds * p.config.MinRate
	p.currentSurplus -= elapsedSeconds * p.config.MaxRate

	// Apply upper bound to debt and surplus. This has to be done early, because conceptually debt and surplus are
	// things which accumulated over time, in the past. So, conceptually, hitting the limit is something which happened
	// in the past and should be enacted before counting the current call in the debt calculation.
	if p.currentDebt > float64(p.config.RateDebtLimit) {
		p.currentDebt = float64(p.config.RateDebtLimit)
	}
	if p.currentSurplus < 0 {
		p.currentSurplus = 0
	}

	// Decide whether to allow a scrape, and reflect the decision upon debt and surplus.
	var isAllowedToScrape bool
	if p.currentSurplus <= float64(p.config.RateSurplusLimit-1) && (p.currentDebt >= 1 || isEagerToScrape) {
		p.currentDebt--
		p.currentSurplus++
		isAllowedToScrape = true
	} else {
		isAllowedToScrape = false
	}

	// Now that we've reflected all effects in the current frame upon debt and surplus, apply bounds.
	if p.currentDebt < 0 {
		p.currentDebt = 0
	}
	if p.currentSurplus > float64(p.config.RateSurplusLimit) {
		p.currentSurplus = float64(p.config.RateSurplusLimit)
	}

	return isAllowedToScrape
}

//#region Test isolation

// pacemakerTestIsolation contains all points of indirection necessary to isolate static function calls
// in the pacemaker unit during tests
type pacemakerTestIsolation struct {
	// Points to [time.Now]
	TimeNow func() time.Time
}

//#endregion Test isolation
