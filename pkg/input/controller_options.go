// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input

import (
	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ControllerOptions are command line options that can be set for controller.Options.
type ControllerOptions struct {
	// MaxConcurrentReconciles are the maximum concurrent reconciles.
	MaxConcurrentReconciles int

	config *ControllerConfig
}

// AddFlags implements Flagger.AddFlags.
func (c *ControllerOptions) AddFlags(fs *pflag.FlagSet, prefix string) {
	fs.IntVar(&c.MaxConcurrentReconciles, prefix+"max-concurrent-reconciles", c.MaxConcurrentReconciles, "The maximum number of concurrent reconciliations.")
}

// Complete implements Completer.Complete.
func (c *ControllerOptions) Complete() error {
	c.config = &ControllerConfig{
		MaxConcurrentReconciles: c.MaxConcurrentReconciles,
	}
	return nil
}

// Completed returns the completed ControllerConfig. Only call this if `Complete` was successful.
func (c *ControllerOptions) Completed() *ControllerConfig {
	return c.config
}

// ControllerConfig is a completed controller configuration.
type ControllerConfig struct {
	// MaxConcurrentReconciles is the maximum number of concurrent reconciles.
	MaxConcurrentReconciles int
}

// Apply sets the values of this ControllerConfig in the given AddOptions.
func (c *ControllerConfig) Apply(opts *controller.Options) {
	opts.MaxConcurrentReconciles = c.MaxConcurrentReconciles
}
