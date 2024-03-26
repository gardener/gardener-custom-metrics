package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/version"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	kmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-custom-metrics/pkg/app"
	"github.com/gardener/gardener-custom-metrics/pkg/ha"
	"github.com/gardener/gardener-custom-metrics/pkg/input"
	"github.com/gardener/gardener-custom-metrics/pkg/metrics_provider"
	gutil "github.com/gardener/gardener-custom-metrics/pkg/util/gardener"
	k8sclient "github.com/gardener/gardener-custom-metrics/pkg/util/k8s/client"
)

func main() {
	rootCmd := getRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// getRootCommand returns the entry point of the application, in the form of a [cobra.Command].
func getRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: app.Name,
		Long: "Gardener custom metrics server. Serves K8s custom metrics for a Gardener seed, based on data retrieved " +
			"by directly scraping metrics from individual shoot kube-apiserver pods.",
	}
	cmd.AddCommand(getVersionCommand())

	// Prepare CLI options for the services implementing the back end
	inputCLIOptions := input.NewCLIOptions()
	// The metrics server library requires that the MetricsProviderService instance processes its own CLI options
	metricsProviderService := metrics_provider.NewMetricsProviderService()
	appOptions := &app.CLIOptions{
		ManagerOptions: gutil.ManagerOptions{
			LeaderElection:          true,
			LeaderElectionID:        gutil.LeaderElectionNameID(app.Name),
			LeaderElectionNamespace: os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		RestOptions: gutil.NewRESTOptions(),
		LogLevel:    app.VerbosityVerbose - 1, // Log everything up to, but excluding verbose
	}

	// Bind CLI option objects to the command line
	inputCLIOptions.AddFlags(cmd.Flags())
	metricsProviderService.AddCLIFlags(cmd.Flags())
	appOptions.AddFlags(cmd.Flags())
	cmd.Flags().AddGoFlagSet(flag.CommandLine) // Make sure we get the klog flags

	cmd.Run = func(cmd *cobra.Command, args []string) {
		runApplication(inputCLIOptions, metricsProviderService, appOptions)
	}

	return cmd
}

// completeAppCLIOptions completes initialisation based on application-level CLI options.
// Upon error, any of the returned Logger, Manager, and HAService may be nil.
func completeAppCLIOptions(
	ctx context.Context, appOptions *app.CLIOptions) (*logr.Logger, kmgr.Manager, *ha.HAService, error) {

	if err := appOptions.Complete(); err != nil {
		return nil, nil, nil, fmt.Errorf("completing application level CLI options: %w", err)
	}

	// Create log
	log := initLogs(ctx, appOptions.Completed().LogLevel)
	log.V(app.VerbosityInfo).Info("Initializing", "version", version.Get().GitVersion)

	// Create manager
	log.V(app.VerbosityInfo).Info("Creating client set")
	if _, err := k8sclient.GetClientSet(appOptions.RestOptions.Kubeconfig); err != nil {
		return &log, nil, nil, fmt.Errorf("create client set: %w", err)
	}
	log.V(app.VerbosityVerbose).Info("Creating controller manager")
	manager, err := kmgr.New(appOptions.RestOptions.Completed().Config, appOptions.Completed().ManagerOptions())
	if err != nil {
		return &log, nil, nil, fmt.Errorf("creating controller manager: %w", err)
	}

	// Create HA service
	haService := ha.NewHAService(manager, appOptions.Namespace, appOptions.AccessIPAddress, appOptions.AccessPort, log)

	return &log, manager, haService, nil
}

// completeInputServiceCLIOptions completes initialisation based on CLI options related to input data processing.
func completeInputServiceCLIOptions(options *input.CLIOptions, log logr.Logger) (input.InputDataService, error) {
	if err := options.Complete(); err != nil {
		return nil, fmt.Errorf("completing input data service CLI options: %w", err)
	}
	inputService := input.NewInputDataServiceFactory().NewInputDataService(options.Completed(), log)

	return inputService, nil
}

// completeMetircsProviderServiceCLIOptions completes initialisation based on CLI options related to metrics serving.
// It returns a [kmgr.Runnable] which can be executed under the supervision of a controller manager.
//
// The onFailedFunc parameter is a function which will be called by the [kmgr.Runnable] if it fails.
func completeMetircsProviderServiceCLIOptions(
	metricsService *metrics_provider.MetricsProviderService,
	inputService input.InputDataService,
	log logr.Logger,
	onFailedFunc context.CancelFunc) (kmgr.RunnableFunc, error) {

	if err := metricsService.CompleteCLIConfiguration(inputService.DataSource(), log); err != nil {
		return nil, fmt.Errorf("configure metrics adapter based on command line arguments: %w", err)
	}

	var metricsProviderRunnable kmgr.RunnableFunc = func(ctx context.Context) error {
		if err := metricsService.Run(ctx.Done()); err != nil {
			log.V(app.VerbosityError).Error(err, "Failed to run custom metrics adapter")
			onFailedFunc()
			return err
		}
		log.Info("Metrics provider service exited")
		return nil
	}

	return metricsProviderRunnable, nil
}

// runApplication implements the activity of the application's main command. As input, it takes various CLI options
// which have been bound to CLI parameters, but not yet completed.
func runApplication(
	inputCLIOptions *input.CLIOptions,
	metricsProviderService *metrics_provider.MetricsProviderService,
	appOptions *app.CLIOptions) {

	ctx := genericapiserver.SetupSignalContext() // Context closed on SIGTERM and SIGINT
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	plog, manager, haService, err := completeAppCLIOptions(ctx, appOptions)
	if err != nil {
		if plog != nil {
			plog.V(app.VerbosityError).Error(err, "Failed to complete app-level CLI options")
		} else {
			fmt.Println(err)
		}
		return
	}
	defer logs.FlushLogs()

	log := *plog
	inputService, err := completeInputServiceCLIOptions(inputCLIOptions, log)
	if err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to complete input service CLI options")
		return
	}

	metricsProviderRunnable, err :=
		completeMetircsProviderServiceCLIOptions(metricsProviderService, inputService, log, cancel)
	if err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to complete metrics provider service CLI options")
		return
	}

	// Add backend services to the manager
	if err := manager.Add(metricsProviderRunnable); err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to add metrics provider service to manager")
		return
	}
	if err := manager.Add(haService); err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to add HA service to manager")
		return
	}
	if err := inputService.AddToManager(manager); err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to add input data service to manager")
		return
	}

	// Finally, run the manager
	log.V(app.VerbosityInfo).Info("Starting controller manager")
	if err := manager.Start(ctx); err != nil {
		log.V(app.VerbosityError).Error(err, "Failed to start the controller manager")
		return
	}
}

func getVersionCommand() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:  "version",
			Long: "Get detailed version and build information",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Println(version.Get())
			},
		}
	)
	return cmd
}

func initLogs(ctx context.Context, level int) logr.Logger {
	logs.InitLogs()

	logger := zap.New(zap.UseDevMode(true), zap.Level(zapcore.Level(-level)))
	logf.SetLogger(logger)
	log := logf.Log.WithName(app.Name)
	logf.IntoContext(ctx, log)

	return log
}
