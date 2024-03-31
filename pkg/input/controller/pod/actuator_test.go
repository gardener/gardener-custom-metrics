// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pod

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/gardener/gardener-custom-metrics/pkg/input/controller/test_util"
	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

var _ = Describe("input.controller.pod.actuator", func() {
	const (
		testNs      = "shoot--my-shoot"
		testPodName = "my-pod"
		testIP      = "192.168.1.1"
	)

	var (
		newTestActuator = func() (*actuator, input_data_registry.InputDataRegistry) {
			idr := input_data_registry.NewInputDataRegistry(1*time.Second, logr.Discard())
			client := test_util.NewFakeClient()
			actuator := NewActuator(client, idr, logr.Discard()).(*actuator)
			return actuator, idr
		}
		newTestPod = func() *corev1.Pod {
			return &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNs,
					Name:      testPodName,
					Labels:    map[string]string{"app": "kubernetes", "role": "apiserver"},
				},
				Status: corev1.PodStatus{
					PodIP: testIP,
				},
			}
		}
	)

	Describe("CreateOrUpdate", func() {
		It("should create a Kapi record, if one does not exist", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()

			// Act
			actuator.CreateOrUpdate(ctx, pod)

			// Assert
			kapi := idr.GetKapiData(testNs, testPodName)
			Expect(kapi).NotTo(BeNil())
			Expect(kapi.PodLabels).To(Equal(pod.Labels))
			Expect(kapi.PodUID).To(Equal(pod.UID))
			Expect(kapi.MetricsUrl).To(Equal(fmt.Sprintf("https://%s/metrics", pod.Status.PodIP)))
			Expect(kapi.MetricsTimeNew).To(BeZero())
			Expect(kapi.MetricsTimeOld).To(BeZero())
			Expect(kapi.TotalRequestCountNew).To(BeZero())
			Expect(kapi.TotalRequestCountOld).To(BeZero())
			Expect(kapi.LastMetricsScrapeTime).To(BeZero())
			Expect(kapi.FaultCount).To(BeZero())
		})
		It("should return no error, and a zero requeue delay, upon successful Kapi creation", func() {
			// Arrange
			actuator, _ := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()

			// Act
			requeue, err := actuator.CreateOrUpdate(ctx, pod)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
		})
		It("should update the recorded metrics URI, labels, and UID, if a Kapi record already exists", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			uid := types.UID("no-uid")
			labels := map[string]string{"dummykey": "dummyvalue"}
			url := "no-url"
			idr.SetKapiData(testNs, testPodName, uid, labels, url)

			// Act
			actuator.CreateOrUpdate(ctx, pod)

			// Assert
			kapi := idr.GetKapiData(testNs, testPodName)
			Expect(kapi).NotTo(BeNil())
			Expect(kapi.PodLabels).To(Equal(pod.Labels))
			Expect(kapi.PodUID).To(Equal(pod.UID))
			Expect(kapi.MetricsUrl).To(Equal(fmt.Sprintf("https://%s/metrics", pod.Status.PodIP)))
		})
		It("should return no error, and a zero requeue delay, upon successful Kapi update", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			idr.SetKapiData(testNs, testPodName, "", nil, "")

			// Act
			requeue, err := actuator.CreateOrUpdate(ctx, pod)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
		})
		It("should leave unrelated fields of the Kapi record unchanged, if a Kapi record already exists", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			idr.SetKapiData(testNs, testPodName, "", nil, "")
			scrapeTimeInitial := time.Now().Add(-1 * time.Minute)
			idr.SetKapiLastScrapeTime(testNs, testPodName, scrapeTimeInitial)
			idr.SetKapiMetrics(testNs, testPodName, 777)
			metricsTimeInitial := time.Now()
			idr.NotifyKapiMetricsFault(testNs, testPodName)
			time.Sleep(1 * time.Millisecond)

			// Act
			requeue, err := actuator.CreateOrUpdate(ctx, pod)

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
			kapi := idr.GetKapiData(testNs, testPodName)
			Expect(kapi).NotTo(BeNil())
			Expect(kapi.MetricsTimeNew.After(metricsTimeInitial)).To(BeFalse())
			Expect(kapi.MetricsTimeOld).To(BeZero())
			Expect(kapi.TotalRequestCountNew).To(Equal(int64(777)))
			Expect(kapi.TotalRequestCountOld).To(BeZero())
			Expect(kapi.LastMetricsScrapeTime).To(Equal(scrapeTimeInitial))
			Expect(kapi.FaultCount).To(Equal(1))
		})
		It("should delete the existing record, if a pod loses the labeling which qualifies it as Kapi pod", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			actuator.CreateOrUpdate(ctx, pod)
			Expect(idr.GetKapiData(testNs, testPodName)).ToNot(BeNil())
			pod.Labels = nil

			// Act
			actuator.CreateOrUpdate(ctx, pod)

			// Assert
			Expect(idr.GetKapiData(testNs, testPodName)).To(BeNil())
		})
	})
	Describe("Delete", func() {
		It("should delete the respective Kapi record, and return no error and zero requeue delay, if the Kapi record exists", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			actuator.CreateOrUpdate(ctx, pod)
			Expect(idr.GetKapiData(testNs, testPodName)).ToNot(BeNil())
			pod.Labels = nil

			// Act
			requeue, err :=
				actuator.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
			Expect(idr.GetKapiData(testNs, testPodName)).To(BeNil())
		})
		It("should return no error, and a zero requeue delay, if the respective Kapi record is missing", func() {
			// Arrange
			actuator, idr := newTestActuator()
			pod := newTestPod()
			ctx := context.Background()
			pod.Labels = nil

			// Act
			requeue, err :=
				actuator.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: testNs, Name: testPodName}})

			// Assert
			Expect(err).To(Succeed())
			Expect(requeue).To(BeZero())
			Expect(idr.GetKapiData(testNs, testPodName)).To(BeNil())
		})
	})
})
