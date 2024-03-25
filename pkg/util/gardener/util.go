// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package gardener provides utilities related to Gardener specifics.
package gardener

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// IsShootNamespace determines whether the format of specified name implies that it is a shoot namespace in a seed
// cluster
func IsShootNamespace(namespace string) bool {
	return strings.HasPrefix(namespace, "shoot-")
}

// WatchBuilder holds various functions which add watch controls to the passed Controller.
type WatchBuilder []func(controller.Controller) error

// Register adds a function which add watch controls to the passed Controller to the WatchBuilder.
func (w *WatchBuilder) Register(funcs ...func(controller.Controller) error) {
	*w = append(*w, funcs...)
}

// AddToController adds the registered watches to the passed controller.
func (w *WatchBuilder) AddToController(ctrl controller.Controller) error {
	for _, f := range *w {
		if err := f(ctrl); err != nil {
			return err
		}
	}
	return nil
}
