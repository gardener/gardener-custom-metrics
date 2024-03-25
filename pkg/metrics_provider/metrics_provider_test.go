// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics_provider

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	mxprov "sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
	"github.com/gardener/gardener-custom-metrics/pkg/util/testutil"
)

var _ = Describe("MetricsProvider", func() {
	const (
		testNs         = "shoot--my-shoot"
		testPodName    = "my-pod"
		testUID        = "my-uid"
		testLabel      = "my-label"
		testLabelValue = "my-label-value"
	)
	var (
		metricInfo = mxprov.CustomMetricInfo{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Namespaced:    true,
			Metric:        metricName,
		}
	)

	Describe("GetMetricByName", func() {
		It("should return nothing if there are no Kapis", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)

			// Act
			metricValue, err := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName}, metricInfo, nil)

			// Assert
			Expect(err).To(Succeed())
			Expect(metricValue).To(BeNil())
		})

		It("should return metrics for the Kapi pod specified by the namespaced name", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)
			idr.SetKapiData(testNs, testPodName, testUID, nil, "")
			idr.SetKapiData(testNs, testPodName+"2", "", nil, "")
			idr.SetKapiMetricsWithTime(testNs, testPodName, 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName, 20, testutil.NewTime(1, 1, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 100, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 120, testutil.NewTime(1, 1, 0))
			provider.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 1, 10)

			// Act
			val, err := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName}, metricInfo, nil)

			// Assert
			Expect(err).To(Succeed())
			Expect(val.Metric.Name).To(Equal(metricName))
			Expect(val.Value.AsApproximateFloat64()).To(Equal(float64(10*1000/60) / 1000))
			Expect(*val.WindowSeconds).To(Equal(int64(60)))
			Expect(val.DescribedObject.Name).To(Equal(testPodName))
			Expect(val.DescribedObject.Namespace).To(Equal(testNs))
			Expect(val.DescribedObject.UID).To(Equal(types.UID(testUID)))
			Expect(val.DescribedObject.APIVersion).To(Equal("v1"))
			Expect(val.DescribedObject.Kind).To(Equal("Pod"))
		})

		It("should respect maxSampleAge", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)
			idr.SetKapiData(testNs, testPodName, testUID, nil, "")
			idr.SetKapiData(testNs, testPodName+"2", "", nil, "")
			idr.SetKapiMetricsWithTime(testNs, testPodName, 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName, 20, testutil.NewTime(1, 1, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 10, testutil.NewTime(1, 0, 1))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 20, testutil.NewTime(1, 1, 1))
			provider.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 2, 31)

			// Act
			valExpired, errExpired := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName}, metricInfo, nil)
			valStillGood, errStillGood := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName + "2"}, metricInfo, nil)

			// Assert
			Expect(errExpired).To(Succeed())
			Expect(errStillGood).To(Succeed())
			Expect(valExpired).To(BeNil())
			Expect(valStillGood).NotTo(BeNil())
			Expect(valStillGood.DescribedObject.Name).To(Equal(testPodName + "2"))
		})

		It("should respect maxSampleGap", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)
			idr.SetKapiData(testNs, testPodName, testUID, nil, "")
			idr.SetKapiData(testNs, testPodName+"2", "", nil, "")
			idr.SetKapiMetricsWithTime(testNs, testPodName, 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName, 20, testutil.NewTime(1, 10, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 20, testutil.NewTime(1, 10, 1))
			provider.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 11, 0)

			// Act
			valGood, errGood := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName}, metricInfo, nil)
			valExcessiveGap, errExcessiveGap := provider.GetMetricByName(
				context.Background(), types.NamespacedName{Namespace: testNs, Name: testPodName + "2"}, metricInfo, nil)

			// Assert
			Expect(errGood).To(Succeed())
			Expect(errExcessiveGap).To(Succeed())
			Expect(valExcessiveGap).To(BeNil())
			Expect(valGood).NotTo(BeNil())
			Expect(valGood.DescribedObject.Name).To(Equal(testPodName))
		})
	})

	Describe("GetMetricBySelector", func() {
		It("should return nothing if there are no Kapis", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)

			// Act
			metricValue, err := provider.GetMetricBySelector(
				context.Background(), testNs, labels.Everything(), metricInfo, nil)

			// Assert
			Expect(err).To(Succeed())
			Expect(metricValue).NotTo(BeNil())
			Expect(metricValue.Items).To(HaveLen(0))
		})

		It("should return only metrics for Kapi pods which match the selector", func() {
			// Arrange
			idr := input_data_registry.FakeInputDataRegistry{}
			provider := NewMetricsProvider(idr.DataSource(), 90*time.Second, 10*time.Minute)
			idr.SetKapiData(testNs, testPodName, testUID, map[string]string{testLabel: testLabelValue}, "")
			idr.SetKapiData(testNs, testPodName+"2", "", nil, "")
			idr.SetKapiMetricsWithTime(testNs, testPodName, 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName, 20, testutil.NewTime(1, 1, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 10, testutil.NewTime(1, 0, 0))
			idr.SetKapiMetricsWithTime(testNs, testPodName+"2", 20, testutil.NewTime(1, 1, 0))
			provider.testIsolation.TimeNow = testutil.NewTimeNowStub(1, 2, 0)
			podSelector, _ := labels.Parse(testLabel + "=" + testLabelValue)

			// Act
			metricList, err := provider.GetMetricBySelector(context.Background(), testNs, podSelector, metricInfo, nil)

			// Assert
			Expect(err).To(Succeed())
			Expect(metricList.Items).To(HaveLen(1))

			val := metricList.Items[0]
			Expect(val.Metric.Name).To(Equal(metricName))
			Expect(val.Value.AsApproximateFloat64()).To(Equal(float64(10*1000/60) / 1000))
			Expect(*val.WindowSeconds).To(Equal(int64(60)))
			Expect(val.DescribedObject.Name).To(Equal(testPodName))
			Expect(val.DescribedObject.Namespace).To(Equal(testNs))
			Expect(val.DescribedObject.UID).To(Equal(types.UID(testUID)))
			Expect(val.DescribedObject.APIVersion).To(Equal("v1"))
			Expect(val.DescribedObject.Kind).To(Equal("Pod"))
		})
	})
})
