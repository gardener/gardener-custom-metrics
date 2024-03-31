// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package input_data_registry

import (
	"crypto/x509"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("input.input_data_registry", func() {
	const (
		nsName          = "MyNs"
		podName         = "MyPod"
		podUid          = types.UID("pod-uid")
		metricsURL      = "https://host:123/metrics"
		shootAuthSecret = "MyShootAuthSecret" //nolint:gosec
	)

	var (
		log         = logr.Discard()
		shootCACert = testutil.GetExampleCACert(0)
	)
	var (
		newPodLabels = func() map[string]string {
			return map[string]string{
				"k1": "v1",
			}
		}
		newInputDataRegistry = func() *inputDataRegistry {
			return NewInputDataRegistry(time.Minute, log).(*inputDataRegistry)
		}
	)

	Describe("NewInputDataRegistry", func() {
		It("should return a properly initialised object", func() {
			idr := newInputDataRegistry()
			Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
			Expect(idr.GetShootCACertificate(nsName)).To(BeNil())
			Expect(idr.GetShootAuthSecret(nsName)).To(BeEmpty())
		})
	})
	Describe("DataSource", func() {
		It("should return a gateway to the same data, and not to a copy", func() {
			// Arrange
			labels := newPodLabels()
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

			// Act
			ds := idr.DataSource()
			idr.SetKapiMetrics(nsName, podName, 42)
			kapis := ds.GetShootKapis(nsName)

			// Assert
			Expect(kapis).To(HaveLen(1))
			Expect(kapis[0].PodName()).To(Equal(podName))
			Expect(kapis[0].PodLabels()).To(Equal(labels))
			Expect(kapis[0].TotalRequestCountNew()).To(Equal(int64(42)))
			Expect(kapis[0].ShootNamespace()).To(Equal(nsName))
			Expect(kapis[0].PodUID()).To(Equal(podUid))
		})
	})
	Describe("GetKapiData", func() {
		Context("when called for a non-existent kapi", func() {
			It("should return nil", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				kapi := idr.GetKapiData(nsName, podName)

				// Assert
				Expect(kapi).To(BeNil())
			})
			It("should not create the kapi", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				idr.GetKapiData(nsName, podName)

				// Assert
				Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
			})
		})
		It("should return a kapi containing the correct values", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.SetKapiMetrics(nsName, podName, 42)

			// Act
			res := idr.GetKapiData(nsName, podName)

			// Assert
			Expect(res).NotTo(BeNil())
			Expect(res.PodName()).To(Equal(podName))
			Expect(res.PodLabels).To(Equal(labels))
			Expect(res.TotalRequestCountNew).To(Equal(int64(42)))
			Expect(res.ShootNamespace()).To(Equal(nsName))
			Expect(res.PodUID).To(Equal(podUid))
			Expect(res.MetricsUrl).To(Equal(metricsURL))
		})
		It("should return a copy, which can be modified by the caller without affecting the registry data", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

			// Act
			res := idr.GetKapiData(nsName, podName)
			res.PodUID = ""

			// Assert
			Expect(idr.GetKapiData(nsName, podName).PodUID).To(Equal(podUid))
		})
	})
	Describe("SetKapiData", func() {
		Context("when called for a non-existent kapi", func() {
			It("should create it with correct contents", func() {
				// Arrange
				idr := newInputDataRegistry()
				labels := newPodLabels()

				// Act
				idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

				// Assert
				res := idr.GetKapiData(nsName, podName)
				Expect(res).NotTo(BeNil())
				Expect(res.ShootNamespace()).To(Equal(nsName))
				Expect(res.PodName()).To(Equal(podName))
				Expect(res.PodUID).To(Equal(podUid))
				Expect(res.PodLabels).To(Equal(labels))
				Expect(res.MetricsUrl).To(Equal(metricsURL))
				Expect(res.MetricsTimeOld).To(Equal(time.Time{}))
				Expect(res.MetricsTimeNew).To(Equal(time.Time{}))
				Expect(res.TotalRequestCountOld).To(Equal(int64(0)))
				Expect(res.TotalRequestCountNew).To(Equal(int64(0)))
				Expect(res.FaultCount).To(Equal(0))
				Expect(res.LastMetricsScrapeTime).To(Equal(time.Time{}))
			})
			It("should deliver exactly one notification - a creation of the kapi with correct values", func() {
				// Arrange
				idr := newInputDataRegistry()
				labels := newPodLabels()
				eventWatcher := newMockWatcher()
				idr.AddKapiWatcher(&eventWatcher.Watcher, false)

				// Act
				idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

				// Assert
				Expect(eventWatcher.EventTypes).To(HaveLen(1))
				Expect(eventWatcher.EventTypes[0]).To(Equal(KapiEventCreate))
				Expect(eventWatcher.EventKapis[0].ShootNamespace()).To(Equal(nsName))
				Expect(eventWatcher.EventKapis[0].PodName()).To(Equal(podName))
				Expect(eventWatcher.EventKapis[0].PodUID()).To(Equal(podUid))
				Expect(eventWatcher.EventKapis[0].PodLabels()).To(Equal(labels))
			})
		})
		Context("when called for an existing kapi", func() {
			It("updates the relevant fields, and only them", func() {
				// Arrange
				idr := newInputDataRegistry()
				labels := newPodLabels()
				idr.SetKapiData(nsName, podName, "", map[string]string{}, "metricsURL")

				time1 := testutil.NewTime(1, 0, 0)
				var requestCount1 int64 = 41
				idr.testIsolation.TimeNow = func() time.Time { return time1 }
				idr.SetKapiMetrics(nsName, podName, requestCount1)

				time2 := testutil.NewTime(2, 0, 0)
				var requestCount2 int64 = 42
				idr.testIsolation.TimeNow = func() time.Time { return time2 }
				idr.SetKapiMetrics(nsName, podName, requestCount2)

				scrapeTime := testutil.NewTime(3, 0, 0)
				idr.SetKapiLastScrapeTime(nsName, podName, scrapeTime)

				// Act
				idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

				// Assert
				res := idr.GetKapiData(nsName, podName)
				Expect(res).NotTo(BeNil())
				Expect(res.ShootNamespace()).To(Equal(nsName))
				Expect(res.PodName()).To(Equal(podName))
				Expect(res.PodUID).To(Equal(podUid))
				Expect(res.PodLabels).To(Equal(labels))
				Expect(res.MetricsUrl).To(Equal(metricsURL))
				Expect(res.MetricsTimeOld).To(Equal(time1))
				Expect(res.MetricsTimeNew).To(Equal(time2))
				Expect(res.TotalRequestCountOld).To(Equal(requestCount1))
				Expect(res.TotalRequestCountNew).To(Equal(requestCount2))
				Expect(res.FaultCount).To(Equal(0))
				Expect(res.LastMetricsScrapeTime).To(Equal(scrapeTime))

			})
			It("does not deliver any notifications", func() {
				// Arrange
				idr := newInputDataRegistry()
				labels := newPodLabels()
				idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

				eventWatcher := newMockWatcher()
				idr.AddKapiWatcher(&eventWatcher.Watcher, false)

				// Act
				idr.SetKapiData(nsName, podName, podUid, labels, "example.com")

				// Assert
				Expect(eventWatcher.EventTypes).To(BeEmpty())
			})
			It("does not modify shoot values", func() {
				// Arrange
				idr := newInputDataRegistry()
				labels := newPodLabels()
				idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
				idr.SetShootCACertificate(nsName, shootCACert)
				certPool := idr.GetShootCACertificate(nsName)
				idr.SetShootAuthSecret(nsName, shootAuthSecret)

				// Act
				idr.SetKapiData(nsName, podName, podUid, labels, "example.com")

				// Assert
				Expect(idr.GetShootCACertificate(nsName).Equal(certPool)).To(BeTrue())
				Expect(idr.GetShootAuthSecret(nsName)).To(Equal(shootAuthSecret))
			})
		})
	})
	Describe("RemoveKapiData", func() {
		It("should have no effect if the registry contains no such kapi, and the output value should reflect it", func() {
			// Arrange
			idr := newInputDataRegistry()

			// Act
			Expect(idr.RemoveKapiData(nsName, podName)).To(BeFalse())

			// Assert
			Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
			Expect(idr.shoots).To(BeEmpty())
		})
		It("should remove the kapi and the output value should reflect it", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)

			// Act
			Expect(idr.RemoveKapiData(nsName, podName)).To(BeTrue())

			// Assert
			Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
		})
		It("should deliver exactly one notification and it is of type deletion", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			eventWatcher := newMockWatcher()
			idr.AddKapiWatcher(&eventWatcher.Watcher, false)

			// Act
			idr.RemoveKapiData(nsName, podName)

			// Assert
			Expect(eventWatcher.EventTypes).To(HaveLen(1))
			Expect(eventWatcher.EventTypes[0]).To(Equal(KapiEventDelete))
			Expect(eventWatcher.EventKapis[0].PodName()).To(Equal(podName))
			Expect(eventWatcher.EventKapis[0].ShootNamespace()).To(Equal(nsName))
		})
		It("should not remove other kapis in the same shoot", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			podName2 := "pod2"
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.SetKapiData(nsName, podName2, podUid+"2", labels, metricsURL+"2")

			// Act
			Expect(idr.RemoveKapiData(nsName, podName)).To(BeTrue())

			// Assert
			Expect(idr.GetKapiData(nsName, podName2)).NotTo(BeNil())
		})
		It("should remove the shoot if that was the last kapi", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			podName2 := "pod2"
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.SetKapiData(nsName, podName2, podUid+"2", labels, metricsURL+"2")
			Expect(idr.RemoveKapiData(nsName, podName2)).To(BeTrue())

			// Act
			Expect(idr.RemoveKapiData(nsName, podName)).To(BeTrue())

			// Assert
			Expect(idr.shoots).To(HaveLen(0))
		})
	})
	Describe("SetKapiMetrics", func() {
		It("should reset fault count to zero", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(BeZero())
			Expect(idr.NotifyKapiMetricsFault(nsName, podName)).To(Equal(1))
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(Equal(1))

			// Act
			idr.SetKapiMetrics(nsName, podName, 42)

			// Assert
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(BeZero())
		})
		It("should shift values and time as follows: <input>-><new>-><old>-><discard>", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			values := []int64{41, 42, 43}

			// Act and assert
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			idr.SetKapiMetrics(nsName, podName, values[0])
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountOld).To(Equal(int64(0)))
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountNew).To(Equal(values[0]))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeOld).To(Equal(time.Time{}))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeNew).To(Equal(testutil.NewTime(1, 0, 0)))

			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(2, 0, 0)
			idr.SetKapiMetrics(nsName, podName, values[1])
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountOld).To(Equal(values[0]))
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountNew).To(Equal(values[1]))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeOld).To(Equal(testutil.NewTime(1, 0, 0)))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeNew).To(Equal(testutil.NewTime(2, 0, 0)))

			// One more step, just in case zero values have special treatment
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(3, 0, 0)
			idr.SetKapiMetrics(nsName, podName, values[2])
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountOld).To(Equal(values[1]))
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountNew).To(Equal(values[2]))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeOld).To(Equal(testutil.NewTime(2, 0, 0)))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeNew).To(Equal(testutil.NewTime(3, 0, 0)))
		})
		It("should reject samples which are too close in time", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			idr.SetKapiMetrics(nsName, podName, 42)
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 1)

			// Act
			idr.SetKapiMetrics(nsName, podName, 43)

			// Assert
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountOld).To(Equal(int64(0)))
			Expect(idr.GetKapiData(nsName, podName).TotalRequestCountNew).To(Equal(int64(42)))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeOld).To(Equal(time.Time{}))
			Expect(idr.GetKapiData(nsName, podName).MetricsTimeNew).To(Equal(testutil.NewTime(1, 0, 0)))
		})
		It("should not create a new kapi if it is missing", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)

			// Act
			idr.SetKapiMetrics(nsName, podName, 43)

			// Assert
			Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
		})
		It("should not deliver a notification", func() {
			// Arrange
			idr := newInputDataRegistry()
			labels := newPodLabels()
			idr.SetKapiData(nsName, podName, podUid, labels, metricsURL)
			idr.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 0, 0)
			eventWatcher := newMockWatcher()
			idr.AddKapiWatcher(&eventWatcher.Watcher, false)

			// Act
			idr.SetKapiMetrics(nsName, podName, 43)

			// Assert
			Expect(eventWatcher.EventTypes).To(BeEmpty())
		})
	})
	Describe("SetKapiLastScrapeTime", func() {
		It("should set the correct value", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)
			scrapeTime := testutil.NewTime(5, 0, 0)

			// Act
			idr.SetKapiLastScrapeTime(nsName, podName, scrapeTime)

			// Assert
			Expect(idr.GetKapiData(nsName, podName).LastMetricsScrapeTime).To(Equal(scrapeTime))
		})
		It("should have no effect if the kapi is missing", func() {
			// Arrange
			idr := newInputDataRegistry()
			scrapeTime := testutil.NewTime(5, 0, 0)

			// Act
			idr.SetKapiLastScrapeTime(nsName, podName, scrapeTime)

			// Assert
			Expect(idr.GetKapiData(nsName, podName)).To(BeNil())
		})
	})
	Describe("NotifyKapiMetricsFault", func() {
		It("should increment the count and return the new value", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(Equal(0))

			// Act and assert
			res := idr.NotifyKapiMetricsFault(nsName, podName)
			Expect(res).To(Equal(1))
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(Equal(1))
			res = idr.NotifyKapiMetricsFault(nsName, podName)
			Expect(res).To(Equal(2))
			Expect(idr.GetKapiData(nsName, podName).FaultCount).To(Equal(2))
		})
	})
	Describe("GetShootAuthSecret", func() {
		It("should return empty string if shoot is missing", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Act
			res := idr.GetShootAuthSecret("AnotherNS")

			// Assert
			Expect(res).To(Equal(""))
		})
		It("should not create the shoot if it is missing", func() {
			// Arrange
			idr := newInputDataRegistry()

			// Act
			idr.GetShootAuthSecret(nsName)

			// Assert
			Expect(idr.shoots).To(BeEmpty())
		})
		It("should return the last stored value", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)
			idr.SetShootAuthSecret(nsName, shootAuthSecret)

			// Act
			res := idr.GetShootAuthSecret(nsName)

			// Assert
			Expect(res).To(Equal(shootAuthSecret))
		})
	})
	Describe("SetShootAuthSecret", func() {
		Context("when the shoot does not exist", func() {
			It("should store the specified value so it can be retrieved later", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				idr.SetShootAuthSecret(nsName, shootAuthSecret)

				// Assert
				Expect(idr.GetShootAuthSecret(nsName)).To(Equal(shootAuthSecret))
			})
			It("should have no effect if the specified value is empty", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				idr.SetShootAuthSecret(nsName, "")

				// Assert
				Expect(idr.shoots).To(BeEmpty())
			})
		})
		Context("when the shoot already exists", func() {
			It("should store the specified value so it can be retrieved later", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

				// Act
				idr.SetShootAuthSecret(nsName, shootAuthSecret)

				// Assert
				Expect(idr.GetShootAuthSecret(nsName)).To(Equal(shootAuthSecret))
			})
			It("should store an empty value but not delete the shoot if it contains Kapis", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL) // Shoot with non-empty auth secret
				idr.SetShootAuthSecret(nsName, shootAuthSecret)
				idr.SetKapiData(nsName+"2", podName, podUid, nil, metricsURL) // Shoot with empty auth secret

				// Act
				idr.SetShootAuthSecret(nsName, "")
				idr.SetShootAuthSecret(nsName+"2", "")

				// Assert
				Expect(idr.GetShootAuthSecret(nsName)).To(BeEmpty())
				Expect(idr.GetShootAuthSecret(nsName + "2")).To(BeEmpty())
				Expect(idr.GetKapiData(nsName, podName).MetricsUrl).To(Equal(metricsURL))
				Expect(idr.GetKapiData(nsName+"2", podName).MetricsUrl).To(Equal(metricsURL))
			})
			It("should store an empty value but not delete the shoot if it contains other data", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetShootCACertificate(nsName, shootCACert)     // Shoot with non-empty auth secret
				idr.SetShootCACertificate(nsName+"2", shootCACert) // Shoot with empty auth secret
				idr.SetShootAuthSecret(nsName, shootAuthSecret)

				// Act
				idr.SetShootAuthSecret(nsName, "")
				idr.SetShootAuthSecret(nsName+"2", "")

				// Assert
				Expect(idr.GetShootAuthSecret(nsName)).To(BeEmpty())
				Expect(idr.GetShootAuthSecret(nsName + "2")).To(BeEmpty())
				Expect(idr.GetShootCACertificate(nsName)).NotTo(BeNil())
				Expect(idr.GetShootCACertificate(nsName + "2")).NotTo(BeNil())
			})
			It("should remove the shoot if that was the last piece of data", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)     // Shoot with non-empty auth secret
				idr.SetKapiData(nsName+"2", podName, podUid, nil, metricsURL) // Shoot with empty auth secret
				idr.SetShootAuthSecret(nsName, shootAuthSecret)
				idr.RemoveKapiData(nsName, podName)
				idr.RemoveKapiData(nsName+"2", podName)

				// Act
				idr.SetShootAuthSecret(nsName, "")
				idr.SetShootAuthSecret(nsName+"2", "")

				// Assert
				Expect(idr.shoots).To(BeEmpty())
			})
		})
	})
	Describe("GetShootCACertificate", func() {
		It("should return nil if shoot is missing", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Act
			res := idr.GetShootCACertificate("AnotherNS")

			// Assert
			Expect(res).To(BeNil())
		})
		It("should not create the shoot if it is missing", func() {
			// Arrange
			idr := newInputDataRegistry()

			// Act
			idr.GetShootCACertificate(nsName)

			// Assert
			Expect(idr.shoots).To(BeEmpty())
		})
		It("should return the last stored value", func() {
			// Arrange
			idr := newInputDataRegistry()
			idr.SetShootCACertificate(nsName, shootCACert)
			expected := x509.NewCertPool()
			expected.AppendCertsFromPEM(shootCACert)

			// Act
			res := idr.GetShootCACertificate(nsName)

			// Assert
			Expect(res.Equal(expected)).To(BeTrue())
		})
	})
	Describe("SetShootCACertificate", func() {
		Context("when the shoot does not exist", func() {
			It("should store the specified value so it can be retrieved later", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				idr.SetShootCACertificate(nsName, shootCACert)

				// Assert
				Expect(testutil.IsEqualCert(idr.GetShootCACertificate(nsName), shootCACert)).To(BeTrue())
			})
			It("should have no effect if the specified value is empty", func() {
				// Arrange
				idr := newInputDataRegistry()

				// Act
				idr.SetShootCACertificate(nsName, nil)

				// Assert
				Expect(idr.shoots).To(BeEmpty())
			})
		})
		Context("when the shoot already exists", func() {
			It("should store the specified value so it can be retrieved later", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

				// Act
				idr.SetShootCACertificate(nsName, shootCACert)

				// Assert
				Expect(testutil.IsEqualCert(idr.GetShootCACertificate(nsName), shootCACert)).To(BeTrue())
			})
			It("should store an empty value but not delete the shoot if it contains Kapis", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL) // Shoot with non-empty cert
				idr.SetShootCACertificate(nsName, shootCACert)
				idr.SetKapiData(nsName+"2", podName, podUid, nil, metricsURL) // Shoot with empty cert

				// Act
				idr.SetShootCACertificate(nsName, nil)
				idr.SetShootCACertificate(nsName+"2", nil)

				// Assert
				Expect(idr.GetShootCACertificate(nsName)).To(BeNil())
				Expect(idr.GetShootCACertificate(nsName + "2")).To(BeNil())
				Expect(idr.GetKapiData(nsName, podName).MetricsUrl).To(Equal(metricsURL))
				Expect(idr.GetKapiData(nsName+"2", podName).MetricsUrl).To(Equal(metricsURL))
			})
			It("should store an empty value but not delete the shoot if it contains other data", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetShootAuthSecret(nsName, shootAuthSecret)     // Shoot with non-empty CA cert
				idr.SetShootAuthSecret(nsName+"2", shootAuthSecret) // Shoot with empty CA cert
				idr.SetShootCACertificate(nsName, shootCACert)

				// Act
				idr.SetShootCACertificate(nsName, nil)
				idr.SetShootCACertificate(nsName+"2", nil)

				// Assert
				Expect(idr.GetShootCACertificate(nsName)).To(BeNil())
				Expect(idr.GetShootCACertificate(nsName + "2")).To(BeNil())
				Expect(idr.GetShootAuthSecret(nsName)).NotTo(BeEmpty())
				Expect(idr.GetShootAuthSecret(nsName + "2")).NotTo(BeEmpty())
			})
			It("should remove the shoot if that was the last piece of data", func() {
				// Arrange
				idr := newInputDataRegistry()
				idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)     // Shoot with non-empty CA cert
				idr.SetKapiData(nsName+"2", podName, podUid, nil, metricsURL) // Shoot with empty CA cert
				idr.SetShootCACertificate(nsName, shootCACert)
				idr.RemoveKapiData(nsName, podName)
				idr.RemoveKapiData(nsName+"2", podName)

				// Act
				idr.SetShootCACertificate(nsName, nil)
				idr.SetShootCACertificate(nsName+"2", nil)

				// Assert
				Expect(idr.shoots).To(BeEmpty())
			})
		})
	})
	Describe("AddKapiWatcher", func() {
		It("should not notify the watcher of existing objects, if the caller has not requested so", func() {
			// Arrange
			idr := newInputDataRegistry()
			watcher := newMockWatcher()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Act
			idr.AddKapiWatcher(&watcher.Watcher, false)

			// Assert
			Expect(watcher.EventTypes).To(BeEmpty())
		})
		It("should notify the watcher of existing objects, if the caller has requested so", func() {
			// Arrange
			idr := newInputDataRegistry()
			watcher := newMockWatcher()
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)
			idr.SetKapiData(nsName, podName+"2", podUid, nil, metricsURL)

			// Act and assert
			idr.AddKapiWatcher(&watcher.Watcher, true)

			// Assert
			Expect(watcher.EventTypes).To(HaveLen(2))
		})
	})
	Describe("RemoveKapiWatcher", func() {
		It("should remove the specified watcher so it does not receive notifications for subsequent changes", func() {
			// Arrange
			idr := newInputDataRegistry()
			watcher := newMockWatcher()
			idr.AddKapiWatcher(&watcher.Watcher, true)

			// Act
			Expect(idr.RemoveKapiWatcher(&watcher.Watcher)).To(BeTrue())
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Assert
			Expect(watcher.EventTypes).To(BeEmpty())
		})
		It("should have no effect if called for a watcher which is currently not registered", func() {
			// Arrange
			idr := newInputDataRegistry()
			watcher1 := newMockWatcher() // This one gets added and never removed
			watcher2 := newMockWatcher() // This one gets added, then removed twice
			watcher3 := newMockWatcher() // This one never gets added, only removed
			idr.AddKapiWatcher(&watcher1.Watcher, true)
			idr.AddKapiWatcher(&watcher2.Watcher, true)
			Expect(idr.RemoveKapiWatcher(&watcher2.Watcher)).To(BeTrue())

			// Act
			Expect(idr.RemoveKapiWatcher(&watcher2.Watcher)).To(BeFalse())
			Expect(idr.RemoveKapiWatcher(&watcher3.Watcher)).To(BeFalse())
			idr.SetKapiData(nsName, podName, podUid, nil, metricsURL)

			// Assert
			Expect(watcher1.EventTypes).To(HaveLen(1))
			Expect(watcher2.EventTypes).To(BeEmpty())
			Expect(watcher3.EventTypes).To(BeEmpty())
		})
	})
})
