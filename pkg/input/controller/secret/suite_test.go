// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGardenerCustomMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gardener custom metrics test suite")
}

var _ = BeforeSuite(func() {
	DeferCleanup(func() {})
})
