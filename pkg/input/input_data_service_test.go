// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
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
})
