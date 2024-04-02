// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

const (
	scrapePeriodFlagName            = "scrape-period"
	scrapeFlowControlPeriodFlagName = "scrape-flow-control-period"
	minSampleGapFlagName            = "min-sample-gap"
)

// CLIOptions are command line options related to processing the data on which custom metrics are based.
type CLIOptions struct {
	config *CLIConfig // Contains the final, processed values of the options

	// For the meaning of the different option fields, see the CLIConfig type, which mirrors these fields
	ScrapePeriod            time.Duration
	ScrapeFlowControlPeriod time.Duration
	MinSampleGap            time.Duration
}

// NewCLIOptions creates a CLIOptions object with default values
func NewCLIOptions() *CLIOptions {
	return &CLIOptions{
		ScrapePeriod:            60 * time.Second,
		ScrapeFlowControlPeriod: 200 * time.Millisecond,
		MinSampleGap:            10 * time.Second,
	}
}

// AddFlags implements [github.com/gardener/gardener/extensions/pkg/controller/cmd.Flagger.AddFlags].
func (options *CLIOptions) AddFlags(flags *pflag.FlagSet) {
	flags.DurationVar(
		&options.ScrapePeriod,
		scrapePeriodFlagName,
		options.ScrapePeriod,
		fmt.Sprintf("How often do we scrape metrics from the same pod. Default: %d", options.ScrapePeriod))
	flags.DurationVar(
		&options.ScrapeFlowControlPeriod,
		scrapeFlowControlPeriodFlagName,
		options.ScrapeFlowControlPeriod,
		fmt.Sprintf(
			"How often do we adjust the level of parallelism we use for scraping pod metrics. Default: %d",
			options.ScrapeFlowControlPeriod))
	flags.DurationVar(
		&options.MinSampleGap,
		minSampleGapFlagName,
		options.MinSampleGap,
		fmt.Sprintf(
			"If the last two metrics samples are closer in time than this, don't use them to calculate rate. Default: %d",
			options.MinSampleGap))
}

// Complete implements [github.com/gardener/gardener/extensions/pkg/controller/cmd.Completer.Complete].
func (options *CLIOptions) Complete() error {
	options.config = &CLIConfig{
		ScrapePeriod:            options.ScrapePeriod,
		ScrapeFlowControlPeriod: options.ScrapeFlowControlPeriod,
		MinSampleGap:            options.MinSampleGap,
	}
	return nil
}

// Completed returns the final, processed values of the options. Only call this if `Complete` was successful.
func (options *CLIOptions) Completed() *CLIConfig {
	return options.config
}

// CLIConfig is a completed configuration, result of successfully parsing and processing CLI options.
// It contains configuration which directs the processing of data on which custom metrics are based.
type CLIConfig struct {
	ScrapePeriod            time.Duration // How often do we scrape a given pod
	ScrapeFlowControlPeriod time.Duration // How often do we adjust the level of scraping parallelism

	// If two consecutive metrics samples are closer than this, they are considered to not provide sufficient
	// differential (rate) calculation accuracy, and are not used as a pair (each may still be used, paired with other
	// samples).
	MinSampleGap time.Duration
}
