package gardener

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("uti/gardener", func() {
	Describe("IsShootCPNamespace", func() {
		It("should work expected on certain predefined values", func() {
			Expect(IsShootCPNamespace("shoot--my-shoot")).To(BeTrue())
			Expect(IsShootCPNamespace("shoot-my-shoot")).To(BeFalse())
			Expect(IsShootCPNamespace("")).To(BeFalse())
			Expect(IsShootCPNamespace("shoot--my--shoot")).To(BeTrue())
		})
	})
})
