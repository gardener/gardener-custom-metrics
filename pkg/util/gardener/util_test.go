// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gardener

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("uti/gardener", func() {
	Describe("IsShootNamespace", func() {
		It("should work as expected on certain predefined values", func() {
			// Not a valid format from the Gardener perspective, but one we expect the less rigorous check in
			// IsShootNamespace() to accept
			Expect(IsShootNamespace("shoot--my-shoot")).To(BeTrue())
			// Legacy format - some clusters may still use it
			Expect(IsShootNamespace("shoot-my-shoot")).To(BeTrue())
			Expect(IsShootNamespace("")).To(BeFalse())
			Expect(IsShootNamespace("shoot--my--shoot")).To(BeTrue())
		})
	})
})
