// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	gutil "github.com/gardener/gardener-custom-metrics/pkg/util/gardener"
)

const (
	namespaceFlagName       = "namespace"
	accessIPAddressFlagName = "access-ip"
	accessPortFlagName      = "access-port"
	burstFlagName           = "burst"
	qpsFlagName             = "qps"
	logLevelFlagName        = "log-level"
	debugFlagName           = "debug"
)

// CLIOptions are command line options with application-level relevance
type CLIOptions struct {
	gutil.ManagerOptions
	// While CLIOptions is a raw data model for the CLI parameters, the config is a processed version, optimised for
	// use by the rest of the application. It contains the actual configuration settings to be used by the application.
	config *CLIConfig

	// For the meaning of the different option fields, see the CLIConfig type, which mirrors these fields
	Namespace       string
	AccessIPAddress string
	AccessPort      int
	RestOptions     *gutil.RESTOptions
	LogLevel        int
	Debug           bool

	// Queries per second allowed on the client connection to the seed kube-apiserver
	QPS float32
	// Short-term burst allowance for the QPS setting
	Burst int
}

// AddFlags implements Flagger.AddFlags.
func (options *CLIOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&options.Namespace, namespaceFlagName, options.Namespace,
		"The K8s namespace in which this process and associated artefacts belong.")
	flags.StringVar(&options.AccessIPAddress, accessIPAddressFlagName, options.AccessIPAddress,
		fmt.Sprintf(
			"The IP address at which custom metrics from this process can be consumed. "+
				"This is where the custom metrics K8s service forwards traffic to. "+
				"When running in a directly accessible pod, this typically is the pod IP. "+
				"When %s is running where it is not directly accessible to its consumers, "+
				"this is the address of a network traffic forwarder which knows how to reach the running intance.",
			Name))
	flags.IntVar(&options.AccessPort, accessPortFlagName, options.AccessPort,
		fmt.Sprintf(
			"The network port at which custom metrics from this process can be consumed. See the %s parameter.",
			accessIPAddressFlagName))
	flags.IntVar(&options.Burst, burstFlagName, options.Burst,
		"Request throttling for this client: brief request bursts are allowed to exceed the throttling rate by this much.")
	flags.Float32Var(&options.QPS, qpsFlagName, options.QPS,
		"Request throttling rate for this client, expressed as average number of requests per second.")
	flags.IntVar(&options.LogLevel, logLevelFlagName, options.LogLevel,
		"Log messages which have their level greater than this, will be suppressed.")
	flags.BoolVar(&options.Debug, debugFlagName, options.Debug,
		"If set, runs the application in a mode which facilitates debugging, e.g. with extremely slow leader election.")
	options.RestOptions.AddFlags(flags)
	options.ManagerOptions.AddFlags(flags)
}

// Complete implements [ctlcmd.Completer.Complete]. It uses CLI parameters to derive the actual configuration settings
// to be used by the application.
func (options *CLIOptions) Complete() error {
	if err := options.ManagerOptions.Complete(); err != nil {
		return err
	}
	if err := options.RestOptions.Complete(); err != nil {
		return err
	}
	options.config = &CLIConfig{
		ManagerConfig:   *options.ManagerOptions.Completed(),
		RESTConfig:      *options.RestOptions.Completed(),
		Namespace:       options.Namespace,
		AccessIPAddress: options.AccessIPAddress,
		AccessPort:      options.AccessPort,
		Debug:           options.Debug,
		LogLevel:        options.LogLevel,
	}
	options.config.RESTConfig.Config.Burst = options.Burst
	options.config.RESTConfig.Config.QPS = options.QPS
	return nil
}

// Completed returns a CLIConfig which contains the configuration settings derived from CLI parameters. Only call this
// if `Complete` was successful.
func (options *CLIOptions) Completed() *CLIConfig {
	return options.config
}

// CLIConfig contains the actual configuration settings to be used by the application. It is a processed version of
// CLIOptions.
type CLIConfig struct {
	gutil.ManagerConfig                  // Configures the controller manager which orchestrates the operation of this program
	RESTConfig          gutil.RESTConfig // Configures access to the seed Kapi

	// The K8s namespace in which this process and associated artefacts belong
	Namespace string
	// The IP address at which custom metrics from this process can be consumed
	AccessIPAddress string
	// The network port at which custom metrics from this process can be consumed
	AccessPort int
	// Log messages which have their level greater than this, will be suppressed
	LogLevel int
	// Run the application in a mode which facilitates debugging, e.g. with extremely slow leader election
	Debug bool
}

// Apply sets the values of this CLIConfig in the given manager.Options.
func (c *CLIConfig) Apply(opts *manager.Options) {
	c.ManagerConfig.Apply(opts)
	opts.LeaderElectionReleaseOnCancel = true

	if c.Debug {
		leaseDuration := time.Second * 600
		renewDeadline := time.Second * 400
		retryPeriod := time.Second * 80
		opts.LeaseDuration = &leaseDuration
		opts.RenewDeadline = &renewDeadline
		opts.RetryPeriod = &retryPeriod
	}
}

// ManagerOptions initializes empty manager.Options, applies the set values and returns it.
func (c *CLIConfig) ManagerOptions() manager.Options {
	var opts manager.Options
	c.Apply(&opts)

	nameRequirement, err := labels.NewRequirement("name", selection.In, []string{"ca", "shoot-access-gardener-custom-metrics"})
	runtime.Must(err)
	secretsLabelSelector := labels.NewSelector().Add(*nameRequirement)

	opts.Cache = cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {
				Label: secretsLabelSelector,
			},
			&corev1.Pod{}: {
				Label: labels.SelectorFromSet(map[string]string{
					"app":  "kubernetes",
					"role": "apiserver",
				}),
			},
		},
	}

	return opts
}
