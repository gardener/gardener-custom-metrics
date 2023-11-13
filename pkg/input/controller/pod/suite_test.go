package pod

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
