// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gardener

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	// LeaderElectionFlag is the name of the command line flag to specify whether to do leader election or not.
	LeaderElectionFlag = "leader-election"
	// LeaderElectionResourceLockFlag is the name of the command line flag to specify the resource type used for leader
	// election.
	LeaderElectionResourceLockFlag = "leader-election-resource-lock"
	// LeaderElectionIDFlag is the name of the command line flag to specify the leader election ID.
	LeaderElectionIDFlag = "leader-election-id"
	// LeaderElectionNamespaceFlag is the name of the command line flag to specify the leader election namespace.
	LeaderElectionNamespaceFlag = "leader-election-namespace"
	// WebhookServerHostFlag is the name of the command line flag to specify the webhook config host for 'url' mode.
	WebhookServerHostFlag = "webhook-config-server-host"
	// WebhookServerPortFlag is the name of the command line flag to specify the webhook server port.
	WebhookServerPortFlag = "webhook-config-server-port"
	// WebhookCertDirFlag is the name of the command line flag to specify the webhook certificate directory.
	WebhookCertDirFlag = "webhook-config-cert-dir"
	// MetricsBindAddressFlag is the name of the command line flag to specify the TCP address that the controller
	// should bind to for serving prometheus metrics.
	// It can be set to "0" to disable the metrics serving.
	MetricsBindAddressFlag = "metrics-bind-address"
	// HealthBindAddressFlag is the name of the command line flag to specify the TCP address that the controller
	// should bind to for serving health probes
	HealthBindAddressFlag = "health-bind-address"

	// KubeconfigFlag is the name of the command line flag to specify a kubeconfig used to retrieve
	// a rest.Config for a manager.Manager.
	KubeconfigFlag = clientcmd.RecommendedConfigPathFlag
	// MasterURLFlag is the name of the command line flag to specify the master URL override for
	// a rest.Config of a manager.Manager.
	MasterURLFlag = "master"
)

// ManagerOptions are command line options that can be set for manager.Options.
type ManagerOptions struct {
	// LeaderElection is whether leader election is turned on or not.
	LeaderElection bool
	// LeaderElectionResourceLock is the resource type used for leader election (defaults to `leases`).
	//
	// When changing the default resource lock, please make sure to migrate via multilocks to
	// avoid situations where multiple running instances of your controller have each acquired leadership
	// through different resource locks (e.g. during upgrades) and thus act on the same resources concurrently.
	// For example, if you want to migrate to the "leases" resource lock, you might do so by migrating
	// to the respective multilock first ("configmapsleases" or "endpointsleases"), which will acquire
	// a leader lock on both resources. After one release with the multilock as a default, you can
	// go ahead and migrate to "leases". Please also keep in mind, that users might skip versions
	// of your controller, so at least add a flashy release note when changing the default lock.
	//
	// Note: before controller-runtime version v0.7, the resource lock was set to "configmaps".
	// Please keep this in mind, when planning a proper migration path for your controller.
	LeaderElectionResourceLock string
	// LeaderElectionID is the id to do leader election with.
	LeaderElectionID string
	// LeaderElectionNamespace is the namespace to do leader election in.
	LeaderElectionNamespace string
	// WebhookServerHost is the host for the webhook server.
	WebhookServerHost string
	// WebhookServerPort is the port for the webhook server.
	WebhookServerPort int
	// WebhookCertDir is the directory that contains the webhook server key and certificate.
	WebhookCertDir string
	// MetricsBindAddress is the TCP address that the controller should bind to for serving prometheus metrics.
	MetricsBindAddress string
	// HealthBindAddress is the TCP address that the controller should bind to for serving health probes.
	HealthBindAddress string

	config *ManagerConfig
}

// AddFlags implements Flagger.AddFlags.
func (m *ManagerOptions) AddFlags(fs *pflag.FlagSet) {
	defaultLeaderElectionResourceLock := m.LeaderElectionResourceLock
	if defaultLeaderElectionResourceLock == "" {
		// explicitly default to leases if no default is specified
		defaultLeaderElectionResourceLock = resourcelock.LeasesResourceLock
	}

	fs.BoolVar(&m.LeaderElection, LeaderElectionFlag, m.LeaderElection, "Whether to use leader election or not when running this controller manager.")
	fs.StringVar(&m.LeaderElectionResourceLock, LeaderElectionResourceLockFlag, defaultLeaderElectionResourceLock, "Which resource type to use for leader election. "+
		"Supported options are 'leases', 'endpointsleases' and 'configmapsleases'.")
	fs.StringVar(&m.LeaderElectionID, LeaderElectionIDFlag, m.LeaderElectionID, "The leader election id to use.")
	fs.StringVar(&m.LeaderElectionNamespace, LeaderElectionNamespaceFlag, m.LeaderElectionNamespace, "The namespace to do leader election in.")
	fs.StringVar(&m.WebhookServerHost, WebhookServerHostFlag, m.WebhookServerHost, "The webhook server host.")
	fs.IntVar(&m.WebhookServerPort, WebhookServerPortFlag, m.WebhookServerPort, "The webhook server port.")
	fs.StringVar(&m.WebhookCertDir, WebhookCertDirFlag, m.WebhookCertDir, "The directory that contains the webhook server key and certificate.")
	fs.StringVar(&m.MetricsBindAddress, MetricsBindAddressFlag, ":8080", "bind address for the metrics server")
	fs.StringVar(&m.HealthBindAddress, HealthBindAddressFlag, ":8081", "bind address for the health server")
}

// Complete implements Completer.Complete.
func (m *ManagerOptions) Complete() error {
	m.config = &ManagerConfig{m.LeaderElection, m.LeaderElectionResourceLock, m.LeaderElectionID, m.LeaderElectionNamespace, m.WebhookServerHost, m.WebhookServerPort, m.WebhookCertDir, m.MetricsBindAddress, m.HealthBindAddress}
	return nil
}

// Completed returns the completed ManagerConfig. Only call this if `Complete` was successful.
func (m *ManagerOptions) Completed() *ManagerConfig {
	return m.config
}

// ManagerConfig is a completed manager configuration.
type ManagerConfig struct {
	// LeaderElection is whether leader election is turned on or not.
	LeaderElection bool
	// LeaderElectionResourceLock is the resource type used for leader election.
	LeaderElectionResourceLock string
	// LeaderElectionID is the id to do leader election with.
	LeaderElectionID string
	// LeaderElectionNamespace is the namespace to do leader election in.
	LeaderElectionNamespace string
	// WebhookServerHost is the host for the webhook server.
	WebhookServerHost string
	// WebhookServerPort is the port for the webhook server.
	WebhookServerPort int
	// WebhookCertDir is the directory that contains the webhook server key and certificate.
	WebhookCertDir string
	// MetricsBindAddress is the TCP address that the controller should bind to for serving prometheus metrics.
	MetricsBindAddress string
	// HealthBindAddress is the TCP address that the controller should bind to for serving health probes.
	HealthBindAddress string
}

// Apply sets the values of this ManagerConfig in the given manager.Options.
func (c *ManagerConfig) Apply(opts *manager.Options) {
	opts.LeaderElection = c.LeaderElection
	opts.LeaderElectionResourceLock = c.LeaderElectionResourceLock
	opts.LeaderElectionID = c.LeaderElectionID
	opts.LeaderElectionNamespace = c.LeaderElectionNamespace
	opts.Metrics = metricsserver.Options{BindAddress: c.MetricsBindAddress}
	opts.HealthProbeBindAddress = c.HealthBindAddress
	opts.WebhookServer = webhook.NewServer(webhook.Options{
		Host:    c.WebhookServerHost,
		Port:    c.WebhookServerPort,
		CertDir: c.WebhookCertDir,
	})
}

// Options initializes empty manager.Options, applies the set values and returns it.
func (c *ManagerConfig) Options() manager.Options {
	var opts manager.Options
	c.Apply(&opts)
	return opts
}

// LeaderElectionNameID returns a leader election ID for the given name.
func LeaderElectionNameID(name string) string {
	return fmt.Sprintf("%s-leader-election", name)
}

// RESTOptions are command line options that can be set for rest.Config.
type RESTOptions struct {
	// Kubeconfig is the path to a kubeconfig.
	Kubeconfig string
	// MasterURL is an override for the URL in a kubeconfig. Only used if out-of-cluster.
	MasterURL string

	config        *RESTConfig
	testIsolation testIsolation
}

// NewRESTOptions creates a new RESTOptions instances
func NewRESTOptions() *RESTOptions {
	return &RESTOptions{
		testIsolation: testIsolation{
			K8sBuildConfigFromFlags: clientcmd.BuildConfigFromFlags,
			K8sInClusterConfig:      rest.InClusterConfig,
			OsGetenv:                os.Getenv,
		},
	}
}

// RESTConfig is a completed REST configuration.
type RESTConfig struct {
	// Config is the rest.Config.
	Config *rest.Config
}

// Enables redirecting library calls, originating in the RESTOptions unit, during test
type testIsolation struct {
	// Points to clientcmd.BuildConfigFromFlags
	K8sBuildConfigFromFlags func(masterUrl, kubeconfigPath string) (*rest.Config, error)
	// Points to rest.InClusterConfig()
	K8sInClusterConfig func() (*rest.Config, error)
	// Points to os.Getenv()
	OsGetenv func(key string) string
}

func (r *RESTOptions) buildConfig() (*rest.Config, error) {
	// If a flag is specified with the config location, use that
	if len(r.Kubeconfig) > 0 {
		return r.testIsolation.K8sBuildConfigFromFlags(r.MasterURL, r.Kubeconfig)
	}
	// If an env variable is specified with the config location, use that
	if kubeconfig := r.testIsolation.OsGetenv(clientcmd.RecommendedConfigPathEnvVar); len(kubeconfig) > 0 {
		return r.testIsolation.K8sBuildConfigFromFlags(r.MasterURL, kubeconfig)
	}
	// If no explicit location, try the in-cluster config
	if c, err := r.testIsolation.K8sInClusterConfig(); err == nil {
		return c, nil
	}

	return r.testIsolation.K8sBuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
}

// Complete implements RESTCompleter.Complete.
func (r *RESTOptions) Complete() error {
	config, err := r.buildConfig()
	if err != nil {
		return err
	}

	r.config = &RESTConfig{config}
	return nil
}

// Completed returns the completed RESTConfig. Only call this if `Complete` was successful.
func (r *RESTOptions) Completed() *RESTConfig {
	return r.config
}

// AddFlags implements Flagger.AddFlags.
func (r *RESTOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&r.Kubeconfig, KubeconfigFlag, "", "Paths to a kubeconfig. Only required if out-of-cluster.")
	fs.StringVar(&r.MasterURL, MasterURLFlag, "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
