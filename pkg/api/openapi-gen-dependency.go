//go:build codegen
// +build codegen

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// A stub to track the build procedure's dependency on openapi-gen
package api

import (
	_ "k8s.io/kube-openapi/cmd/openapi-gen"
)
