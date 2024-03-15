package gardener

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("uti/gardener", func() {
	Describe("IsShootNamespace", func() {
		It("should work expected on certain predefined values", func() {
			Expect(IsShootNamespace("shoot--my-shoot")).To(BeTrue())
			Expect(IsShootNamespace("shoot-my-shoot")).To(BeFalse())
			Expect(IsShootNamespace("")).To(BeFalse())
			Expect(IsShootNamespace("shoot--my--shoot")).To(BeTrue())
		})
	})
})
