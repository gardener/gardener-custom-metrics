// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package input provides shoot kube-apiserver (ShootKapi) application metrics
package input

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	kmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	podctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller/pod"
	secretctl "github.com/gardener/gardener-custom-metrics/pkg/input/controller/secret"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/input/metrics_scraper"
)

// InputDataServiceFactory creates InputDataService instances. It allows replacing certain functions, to support
// test isolation.
type InputDataServiceFactory struct {
	// An indirection for the NewInputDataService function. Allows replacing behavior to provide test isolation.
	newInputDataServiceFunc func(cliConfig *CLIConfig, parentLogger logr.Logger) InputDataService
}

// NewInputDataServiceFactory creates an InputDataServiceFactory instance.
func NewInputDataServiceFactory() *InputDataServiceFactory {
	return &InputDataServiceFactory{newInputDataServiceFunc: newInputDataService}
}

// NewInputDataService creates an InputDataService instance, based on a CLIConfig object which represents command line
// preferences which control behavior.
func (f *InputDataServiceFactory) NewInputDataService(cliConfig *CLIConfig, parentLogger logr.Logger) InputDataService {
	return f.newInputDataServiceFunc(cliConfig, parentLogger)
}

// InputDataService is the main type of the input package. It provides application metrics for the
// kube-apiserver (Kapi) pods of all shoots on a single seed.
//
// To crete instances, use NewInputDataService().
type InputDataService interface {
	// DataSource returns an interface for consuming metrics provided by the InputDataService
	DataSource() input_data_registry.InputDataSource
	// AddToManager adds all of InputDataService's underlying data gathering activities to the specified manager.
	AddToManager(manager kmgr.Manager) error
}

type inputDataService struct {
	// Central data repository, used to synchronize/communicate between the different components of InputDataRegistry,
	// and as a sink for the data output by InputDataRegistry.
	inputDataRegistry input_data_registry.InputDataRegistry

	config *CLIConfig
	log    logr.Logger

	testIsolation testIsolation
}

// NewInputDataService creates an InputDataService instance.
//
// cliConfig contains configurable settings which influence the behavior of the resulting object.
func newInputDataService(cliConfig *CLIConfig, parentLogger logr.Logger) InputDataService {
	log := parentLogger.WithName("input")
	return &inputDataService{
		inputDataRegistry: input_data_registry.NewInputDataRegistry(cliConfig.MinSampleGap, log),
		config:            cliConfig,
		log:               log,
		testIsolation: testIsolation{
			NewScraper: metrics_scraper.NewScraper,
		},
	}
}

func (ids *inputDataService) DataSource() input_data_registry.InputDataSource {
	return ids.inputDataRegistry.DataSource()
}

func (ids *inputDataService) AddToManager(manager kmgr.Manager) error {
	ids.log.V(app.VerbosityInfo).Info("Creating scraper")
	scraper := ids.testIsolation.NewScraper(
		ids.inputDataRegistry,
		ids.config.ScrapePeriod,
		ids.config.ScrapeFlowControlPeriod,
		ids.log.V(1).WithName("scraper"))

	ids.log.V(app.VerbosityVerbose).Info("Updating manager schemes")
	builder := runtime.NewSchemeBuilder(scheme.AddToScheme)
	if err := builder.AddToScheme(manager.GetScheme()); err != nil {
		return fmt.Errorf("add input data service scheme to manager: %w", err)
	}

	ids.log.V(app.VerbosityVerbose).Info("Adding controllers to manager")
	ids.config.PodController.Apply(&podctl.DefaultAddOptions.Controller)
	ids.config.SecretController.Apply(&secretctl.DefaultAddOptions.Controller)

	if err := podctl.AddToManager(manager, ids.inputDataRegistry, ids.log.V(1)); err != nil {
		return fmt.Errorf("add pod controller to manager: %w", err)
	}
	if err := secretctl.AddToManager(manager, ids.inputDataRegistry, ids.log.V(1)); err != nil {
		return fmt.Errorf("add secret controller to manager: %w", err)
	}

	ids.log.V(app.VerbosityVerbose).Info("Adding scraper to manager")
	if err := manager.Add(scraper); err != nil {
		return fmt.Errorf("add scraper to controller manager: %w", err)
	}

	return nil
}

//#region Test isolation

// testIsolation contains all points of indirection necessary to isolate static function calls
// in the InputDataService unit during tests
type testIsolation struct {
	// Forwards call to [metrics_scraper.ScraperFactory.NewScraper]
	NewScraper func(dataRegistry input_data_registry.InputDataRegistry,
		scrapePeriod time.Duration,
		scrapeFlowControlPeriod time.Duration,
		log logr.Logger) *metrics_scraper.Scraper
}

//#endregion Test isolation
