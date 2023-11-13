package metrics_provider

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

var _ = Describe("MetricsService", func() {
	Describe("AddCLIFlags", func() {
		It("should replace the AdapterBase's flag set with the specified one", func() {
			// Arrange
			mps := NewMetricsProviderService()
			flags := pflag.NewFlagSet("", pflag.PanicOnError)

			// Act
			mps.AddCLIFlags(flags)

			// Assert
			Expect(mps.FlagSet == flags).To(BeTrue())
			for _, flagName := range []string{"max-sample-age", "max-sample-gap"} {
				flag := flags.Lookup(flagName)
				Expect(flag).NotTo(BeNil())
				Expect(flag.DefValue).NotTo(BeZero())
			}
		})
	})
	Describe("CompleteCLIConfiguration", func() {
		It("should create a MetricsProvider based on the specified configuration", func() {
			// Arrange
			mps := NewMetricsProviderService()
			var actualDataSource input_data_registry.InputDataSource
			var actualMaxSampleAge, actualMaxSampleGap time.Duration
			mps.testIsolation.NewMetricsProvider =
				func(ds input_data_registry.InputDataSource, msa time.Duration, msg time.Duration) *MetricsProvider {
					actualDataSource = ds
					actualMaxSampleAge = msa
					actualMaxSampleGap = msg
					return nil
				}
			idr := input_data_registry.FakeInputDataRegistry{}
			expectedDataSource := idr.DataSource()

			// Act
			mps.CompleteCLIConfiguration(expectedDataSource, logr.Discard())

			// Assert
			Expect(actualDataSource).To(Equal(expectedDataSource))
			Expect(actualMaxSampleAge).To(Equal(90 * time.Second))
			Expect(actualMaxSampleGap).To(Equal(10 * time.Minute))
			Expect(mps.Name).To(Equal(adapterName))
			Expect(mps.OpenAPIConfig).NotTo(BeNil())
		})
	})
})
