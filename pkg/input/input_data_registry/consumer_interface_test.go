package input_data_registry

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("the input.input_data_registry.InputDataSource implementation", func() {
	const (
		nsName     = "MyNs"
		podName    = "MyPod"
		podUid     = types.UID("pod-uid")
		metricsURL = "https://host:123/metrics"
	)

	var (
		log          = logr.Discard()
		newPodLabels = func() map[string]string {
			return map[string]string{
				"k1": "v1",
			}
		}
		newInputDataRegistry = func() *inputDataRegistry {
			return NewInputDataRegistry(time.Minute, log).(*inputDataRegistry)
		}
	)

	Describe("GetShootKapis", func() {
		It("should return empty collection if there are no shoots in the registry", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()

			// Act
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).To(BeEmpty())
		})
		It("should return empty collection if there are shoots in registry, but the requested shoot is missing", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()
			idr.SetKapiData(nsName+"2", podName, podUid, nil, "dummy")

			// Act
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).To(BeEmpty())
		})
		It("should have no effect if the registry contains no such shoot", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()

			// Act
			ds.GetShootKapis(nsName)

			// Assert
			Expect(idr.shoots).To(BeEmpty())
		})
		It("should return empty collection if the requested shoot is in the registry, but it has no Kapis", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()
			idr.SetShootAuthSecret(nsName, "dummy")

			// Act
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).To(BeEmpty())
		})
		It("should reflect changes which occurred after the DataSource was acquired", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Act
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).NotTo(BeEmpty())
		})
		It("should return one object for each Kapi of the specified shoot, and that object should have the same values as the Kapi", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.SetKapiMetrics(nsName, podName, 42)
			idr.SetKapiData(nsName, podName+"2", podUid+"2", labels, metricsURL+"2")

			// Act
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).To(HaveLen(2))
			Expect(kapis[0].PodName()).To(Equal(podName))
			Expect(kapis[0].PodLabels()).To(Equal(labels))
			Expect(kapis[0].TotalRequestCountNew()).To(Equal(int64(42)))
			Expect(kapis[0].ShootNamespace()).To(Equal(nsName))
			Expect(kapis[0].PodUID()).To(Equal(podUid))
			Expect(kapis[0].MetricsTimeNew()).NotTo(BeZero())
		})
		It("should return objects which capture the state of the Kapis at the time of the call, and are not affected by subsequent changes to the registry", func() {
			// Arrange
			idr := newInputDataRegistry()
			ds := idr.DataSource()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)
			idr.SetKapiMetrics(nsName, podName, 42)

			// Act
			kapis := ds.GetShootKapis(nsName)
			idr.SetKapiMetrics(nsName, podName, 43)

			// Assert
			Expect(kapis[0].TotalRequestCountNew()).To(Equal(int64(42)))
		})
	})
})
