// Package metrics_provider implements a custom metrics server which exposes shoot kube-apiserver pod data available
// in a [input_data_registry.InputDataSource].
package metrics_provider

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	customexternalmetrics "sigs.k8s.io/custom-metrics-apiserver/pkg/apiserver"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"

	generatedopenapi "github.com/gardener/gardener-custom-metrics/pkg/api/generated/openapi"
	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

const (
	adapterName = app.Name
)

// MetricsProviderService is the main type of the package. It runs a custom metrics server, which exposes shoot
// kube-apiserver pod data available in a [input_data_registry.InputDataSource]. No more than one instance of this type
// is meant to exist per process.
type MetricsProviderService struct {
	basecmd.AdapterBase                                     // AdapterBase provides a metrics server framework
	dataSource          input_data_registry.InputDataSource // Contains the data exposed as custom metrics
	log                 logr.Logger

	// The last sample for a pod is valid for this long
	maxSampleAge time.Duration

	// If two consecutive samples are further apart than this, the pair is not considered in rate calculation
	maxSampleGap time.Duration

	testIsolation metricsServiceTestIsolation
}

// NewMetricsProviderService creates a partially initialised MetricsProviderService instance. Initialisation is
// completed via subsequent calls to the AddCLIFlags() and CompleteCLIConfiguration() methods.
func NewMetricsProviderService() *MetricsProviderService {
	result := &MetricsProviderService{
		AdapterBase: basecmd.AdapterBase{
			Name: adapterName,
			OpenAPIConfig: genericapiserver.DefaultOpenAPIConfig(
				generatedopenapi.GetOpenAPIDefinitions,
				openapinamer.NewDefinitionNamer(customexternalmetrics.Scheme)),
		},
		maxSampleAge:  90 * time.Second,
		maxSampleGap:  600 * time.Second,
		testIsolation: metricsServiceTestIsolation{NewMetricsProvider: NewMetricsProvider},
	}
	result.OpenAPIConfig.Info.Title = adapterName
	result.OpenAPIConfig.Info.Version = "1.0.0"

	return result
}

// AddCLIFlags adds to the specified flag set the flags necessary to configure this MetricsProviderService instance.
func (mps *MetricsProviderService) AddCLIFlags(cliFlagSet *pflag.FlagSet) {
	// The call to Flags() below triggers [cmd.AdapterBase]'s flag set initialisation. So [cmd.AdapterBase]'s
	// reference should be pointed to the correct flag set first. If not, [cmd.AdapterBase] will initialize its default
	// flag set instance, which has nothing to do with the actual flag set we're using.
	mps.FlagSet = cliFlagSet

	mps.Flags().DurationVar(
		&mps.maxSampleAge,
		"max-sample-age",
		mps.maxSampleAge,
		fmt.Sprintf(
			"How long will the last metrics sample for a given pod be considered valid, after it is collected. Default: %s",
			mps.maxSampleAge),
	)
	mps.Flags().DurationVar(
		&mps.maxSampleGap,
		"max-sample-gap",
		mps.maxSampleGap,
		fmt.Sprintf(
			"The maximum time between a pair of two consecutive samples, before the pair is considered unsuitable "+
				"for rate calculation. Default: %s",
			mps.maxSampleGap),
	)
}

// CompleteCLIConfiguration sets the logger and dataSource to be used for the rest of the object's lifetime,
// and then completes CLI configuration, applying the CLI options.
// This late configuration (not in constructor) is forced by [cmd.AdapterBase]'s design. It requires early
// instantiation (before CLI configuration has been parsed), so it can do its own CLI parameter processing.
func (mps *MetricsProviderService) CompleteCLIConfiguration(
	dataSource input_data_registry.InputDataSource, parentLogger logr.Logger) error {

	mps.dataSource = dataSource
	mps.log = parentLogger.WithName("metrics-provider").V(1)
	if err := mps.createProvider(); err != nil {
		return fmt.Errorf("creating metrics provider: %w", err)
	}
	return nil
}

// createProvider creates the proper metrics provider - a MetricsProvider instance, and registers it as the metrics
// server's custom metrics handler.
func (mps *MetricsProviderService) createProvider() error {
	mps.WithCustomMetrics(
		mps.testIsolation.NewMetricsProvider(mps.dataSource, mps.maxSampleAge, mps.maxSampleGap))
	return nil
}

// metricsServiceTestIsolation contains all points of indirection necessary to isolate static function calls
// in the MetricsService unit during tests
type metricsServiceTestIsolation struct {
	// Points to NewMetricsProvider
	NewMetricsProvider func(
		dataSource input_data_registry.InputDataSource,
		maxSampleAge time.Duration,
		maxSampleGap time.Duration) *MetricsProvider
}
