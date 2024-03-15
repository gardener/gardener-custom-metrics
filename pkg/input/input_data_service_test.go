package input

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/input/metrics_scraper"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.inputDataService", func() {
	const (
		testScrapePeriod            = 1 * time.Minute
		testScrapeFlowControlPeriod = 20 * time.Millisecond
		testMinSampleGap            = 20 * time.Second
	)

	var (
		newInputDataService = func() (*inputDataService, *input_data_registry.FakeInputDataRegistry) {
			var idr *input_data_registry.FakeInputDataRegistry
			instrumentedFactory := NewInputDataServiceFactory()
			instrumentedFactory.newInputDataServiceFunc =
				func(cliConfig *CLIConfig, parentLogger logr.Logger) InputDataService {

					ids := NewInputDataServiceFactory().NewInputDataService(cliConfig, parentLogger).(*inputDataService)
					idr = &input_data_registry.FakeInputDataRegistry{MinSampleGap: cliConfig.MinSampleGap}
					ids.inputDataRegistry = idr
					return ids
				}
			config := &CLIConfig{
				ScrapePeriod:            testScrapePeriod,
				ScrapeFlowControlPeriod: testScrapeFlowControlPeriod,
				MinSampleGap:            testMinSampleGap,
			}
			return instrumentedFactory.NewInputDataService(config, logr.Discard()).(*inputDataService), idr
		}
	)

	Describe("NewInputDataService", func() {
		It("should configure the input data registry with the specified sample gap", func() {
			// Arrange

			// Act
			ids, _ := newInputDataService()

			// Assert
			Expect(ids.inputDataRegistry.(*input_data_registry.FakeInputDataRegistry).MinSampleGap).To(Equal(testMinSampleGap))
		})
	})

	Describe("DataSource", func() {
		It("should point to the same registry at the one supplied to the scraper", func() {
			// Arrange
			ids, idr := newInputDataService()

			// Act
			result := ids.DataSource()

			// Assert
			idr.SetKapiData("ns", "pod", "", nil, "")
			kapis := result.GetShootKapis("ns")
			Expect(kapis).To(HaveLen(1))
			Expect(kapis[0].PodName()).To(Equal("pod"))
		})
	})

	Describe("AddToManager", func() {
		It("should add the scraper, pod controller, and secret controller to the manager", func() {
			// Arrange
			ids, _ := newInputDataService()
			fm := testutil.NewFakeManager()

			// Act
			result := ids.AddToManager(fm)

			// Assert
			Expect(result).Should(Succeed())
			Expect(testutil.GetRunnables[*metrics_scraper.Scraper](fm)).To(HaveLen(1))
			Expect(testutil.GetRunnables[controller.Controller](fm)).To(HaveLen(2))
		})

		It("should add apimachinery runtime types scheme to the manager", func() {
			// Arrange
			ids, _ := newInputDataService()
			fm := testutil.NewFakeManager()

			// Act
			result := ids.AddToManager(fm)

			// Assert
			Expect(result).To(Succeed())
			runtimeScheme := runtime.NewScheme()
			builder := runtime.NewSchemeBuilder(scheme.AddToScheme)
			builder.AddToScheme(runtimeScheme)
			for gvk := range runtimeScheme.AllKnownTypes() {
				Expect(fm.Scheme.Recognizes(gvk)).To(BeTrue())
			}
		})

		It("should create a new data registry and pass it to the scraper", func() {
			// Arrange
			ids, _ := newInputDataService()
			fm := testutil.NewFakeManager()
			var registryPassedToScraperConstructor input_data_registry.InputDataRegistry
			ids.testIsolation.NewScraper = func(
				dataRegistry input_data_registry.InputDataRegistry,
				scrapePeriod time.Duration,
				scrapeFlowControlPeriod time.Duration,
				log logr.Logger) *metrics_scraper.Scraper {

				registryPassedToScraperConstructor = dataRegistry

				return nil
			}

			// Act
			result := ids.AddToManager(fm)

			// Assert
			Expect(result).To(Succeed())
			Expect(registryPassedToScraperConstructor).NotTo(BeNil())
			Expect(registryPassedToScraperConstructor == ids.inputDataRegistry).To(BeTrue())
		})

		It("should configure the scraper with the specified scrape period and flow control period", func() {
			// Arrange
			ids, _ := newInputDataService()
			fm := testutil.NewFakeManager()
			var scrapePeriodPassedToScraperConstructor time.Duration
			var scrapeFlowControlPeriodPassedToScraperConstructor time.Duration

			ids.testIsolation.NewScraper = func(
				dataRegistry input_data_registry.InputDataRegistry,
				scrapePeriod time.Duration,
				scrapeFlowControlPeriod time.Duration,
				log logr.Logger) *metrics_scraper.Scraper {

				scrapePeriodPassedToScraperConstructor = scrapePeriod
				scrapeFlowControlPeriodPassedToScraperConstructor = scrapeFlowControlPeriod

				return nil
			}

			// Act
			result := ids.AddToManager(fm)

			// Assert
			Expect(result).To(Succeed())
			Expect(scrapePeriodPassedToScraperConstructor).To(Equal(testScrapePeriod))
			Expect(scrapeFlowControlPeriodPassedToScraperConstructor).To(Equal(testScrapeFlowControlPeriod))
		})
	})
})
