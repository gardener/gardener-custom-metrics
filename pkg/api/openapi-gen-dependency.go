//go:build codegen
// +build codegen

// A stub to track the build procedure's dependency on openapi-gen
package api

import (
	_ "k8s.io/kube-openapi/cmd/openapi-gen"
)
