// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics_provider

import (
	"context"
	"fmt"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/custom-metrics-apiserver/pkg/provider"

	"github.com/gardener/gardener-custom-metrics/pkg/input/input_data_registry"
)

const (
	metricName = "shoot:apiserver_request_total:sum"
)

// MetricsProvider implements [provider.CustomMetricsProvider]
type MetricsProvider struct {
	dataSource input_data_registry.InputDataSource

	// The last sample for a pod is valid for this long
	maxSampleAge time.Duration

	// If two consecutive samples are further apart than this, the pair is not considered in rate calculation
	maxSampleGap time.Duration

	testIsolation metricsProviderTestIsolation
}

// NewMetricsProvider creates a MetricsProvider which relies on the specified [input_data_registry.InputDataSource] as
// source of data.
//
// maxSampleAge - If a data sample is older than that, it will not be considered when calculating metrics.
//
// maxSampleGap - When calculating metrics based on difference between two samples, if the samples are further apart
// than this, they will not be considered.
func NewMetricsProvider(
	dataSource input_data_registry.InputDataSource,
	maxSampleAge time.Duration,
	maxSampleGap time.Duration) *MetricsProvider {

	return &MetricsProvider{
		dataSource:    dataSource,
		maxSampleAge:  maxSampleAge,
		maxSampleGap:  maxSampleGap,
		testIsolation: metricsProviderTestIsolation{TimeNow: time.Now},
	}
}

// ListAllMetrics implements [provider.CustomMetricsProvider.ListAllMetrics].
func (mp *MetricsProvider) ListAllMetrics() []provider.CustomMetricInfo {
	return []provider.CustomMetricInfo{
		{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric:        metricName,
			Namespaced:    true,
		},
	}
}

// GetMetricByName implements [provider.CustomMetricsProvider.GetMetricByName].
func (mp *MetricsProvider) GetMetricByName(
	_ context.Context,
	name types.NamespacedName,
	metricInfo provider.CustomMetricInfo,
	_ labels.Selector) (*custom_metrics.MetricValue, error) {

	metrics, err := mp.getMetricByPredicate(
		name.Namespace,
		func(kapi input_data_registry.ShootKapi) bool { return kapi.PodName() == name.Name },
		metricInfo)
	if err != nil {
		return nil, fmt.Errorf("retrieving custom metric %s/%s: %w", name.Namespace, name.Name, err)
	}
	if len(metrics.Items) == 0 {
		return nil, nil
	}
	if len(metrics.Items) > 1 {
		return nil, fmt.Errorf(
			"retrieving custom metric %s/%s: multiple metrics found with that name", name.Namespace, name.Name)
	}
	return &metrics.Items[0], nil
}

// GetMetricBySelector implements [provider.CustomMetricsProvider.GetMetricBySelector].
func (mp *MetricsProvider) GetMetricBySelector(
	_ context.Context,
	namespace string,
	podSelector labels.Selector,
	metricInfo provider.CustomMetricInfo,
	_ labels.Selector) (*custom_metrics.MetricValueList, error) {

	return mp.getMetricByPredicate(
		namespace,
		func(kapi input_data_registry.ShootKapi) bool {
			return podSelector.Matches(labels.Set(kapi.PodLabels()))
		},
		metricInfo)
}

// kapiPredicate is solely used in conjunction with getMetricByPredicate()
type kapiPredicate func(kapi input_data_registry.ShootKapi) bool

// getMetricByPredicate is a somewhat more flexible (filters by arbitrary predicate instead of selector) implementation
// of [provider.CustomMetricsProvider.GetMetricBySelector]
//
// The predicate returns true for [input_data_registry.ShootKapi] instances which should be included in the result.
func (mp *MetricsProvider) getMetricByPredicate(
	namespace string,
	predicate kapiPredicate,
	metricInfo provider.CustomMetricInfo) (*custom_metrics.MetricValueList, error) {

	if metricInfo.Metric != metricName {
		return &custom_metrics.MetricValueList{}, nil
	}

	kapis := mp.dataSource.GetShootKapis(namespace)
	result := &custom_metrics.MetricValueList{}
	for _, kapi := range kapis {
		if !predicate(kapi) {
			continue
		}

		gap := kapi.MetricsTimeNew().Sub(kapi.MetricsTimeOld())
		if gap == 0 {
			// Before actual samples get recorded, the times point to the start of the epoch
			continue
		}
		if gap > mp.maxSampleGap {
			// Too many samples missed between old and new samples. The calculation would be correct, but not relevant
			// enough to the present moment, as it may be applying excessive smoothing to a sharply changing quantity.
			// Also covers the case right after the very first sample gets registered, so the old sample still points
			// to the start of the epoch.
			continue
		}
		if kapi.MetricsTimeNew().Before(mp.testIsolation.TimeNow().Add(-mp.maxSampleAge)) {
			// Samples too old
			continue
		}

		requestRate := float64(kapi.TotalRequestCountNew()-kapi.TotalRequestCountOld()) / gap.Seconds()
		result.Items = append(result.Items, custom_metrics.MetricValue{
			DescribedObject: custom_metrics.ObjectReference{
				Kind:       "Pod",
				Name:       kapi.PodName(),
				Namespace:  kapi.ShootNamespace(),
				APIVersion: "v1",
				UID:        kapi.PodUID(),
			},
			Metric: custom_metrics.MetricIdentifier{
				Name: metricName,
			},
			Value:         *resource.NewMilliQuantity(int64(requestRate*1000), resource.DecimalSI),
			Timestamp:     metav1.Time{Time: kapi.MetricsTimeNew()},
			WindowSeconds: pointer.Int64(int64(math.Round(gap.Seconds()))),
		})
	}

	return result, nil
}

// metricsProviderTestIsolation contains all points of indirection necessary to isolate static function calls
// in the MetricsProvider unit during tests
type metricsProviderTestIsolation struct {
	// Points to [time.Now]
	TimeNow func() time.Time
}
