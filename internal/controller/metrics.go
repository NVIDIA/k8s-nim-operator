/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	appsv1alpha1 "github.com/NVIDIA/k8s-nim-operator/api/apps/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	statusUnknown        = "Unknown"
	nimCacheStatusValues = []string{
		appsv1alpha1.NimCacheStatusFailed,
		appsv1alpha1.NimCacheStatusInProgress,
		appsv1alpha1.NimCacheStatusReady,
		appsv1alpha1.NimCacheStatusNotReady,
		appsv1alpha1.NimCacheStatusPVCCreated,
		appsv1alpha1.NimCacheStatusPending,
		appsv1alpha1.NimCacheStatusStarted,
		statusUnknown,
	}

	nimCacheStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimCache_status_total",
			Help: "Total number of NimCaches with specific status value.",
		},
		[]string{"status"},
	)

	nimServiceStatusValues = []string{
		appsv1alpha1.NIMServiceStatusFailed,
		appsv1alpha1.NIMServiceStatusReady,
		appsv1alpha1.NIMServiceStatusPending,
		appsv1alpha1.NIMServiceStatusNotReady,
		statusUnknown,
	}
	nimServiceStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimService_status_total",
			Help: "Total number of NimServices with specific status value.",
		},
		[]string{"status"},
	)

	nimPipelineStatusValues = []string{
		appsv1alpha1.NIMPipelineStatusFailed,
		appsv1alpha1.NIMPipelineStatusReady,
		appsv1alpha1.NIMPipelineStatusNotReady,
		statusUnknown,
	}
	nimPipelineStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nimPipeline_status_total",
			Help: "Total number of NimPipelines with specific status value.",
		},
		[]string{"status"},
	)

	nemoServiceStatusValues = []string{
		appsv1alpha1.NemoServiceStatusFailed,
		appsv1alpha1.NemoServiceStatusReady,
		appsv1alpha1.NemoServiceStatusNotReady,
		statusUnknown,
	}
	nemoServiceStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemoService_status_total",
			Help: "Total number of NemoServices with specific status value.",
		},
		[]string{"status"},
	)
)

func init() {
	// Register the custom metric with the legacyregistry
	metrics.Registry.MustRegister(nimCacheStatusMetric)
	metrics.Registry.MustRegister(nimServiceStatusMetric)
	metrics.Registry.MustRegister(nimPipelineStatusMetric)
}

func refreshNIMCacheMetrics(nimCacheList *appsv1alpha1.NIMCacheList) {
	nimCacheStatusMetric.WithLabelValues("all").Set(float64(len(nimCacheList.Items)))
	counts := make(map[string]int)
	for _, item := range nimCacheList.Items {
		status := item.Status.State
		if status == "" {
			status = statusUnknown
		}

		if _, ok := counts[status]; !ok {
			counts[status] = 1
		} else {
			counts[status] = counts[status] + 1
		}
	}
	for _, status := range nimCacheStatusValues {
		if _, ok := counts[status]; !ok {
			nimCacheStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nimCacheStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNIMServiceMetrics(nimServiceList *appsv1alpha1.NIMServiceList) {
	nimServiceStatusMetric.WithLabelValues("all").Set(float64(len(nimServiceList.Items)))
	counts := make(map[string]int)
	for _, item := range nimServiceList.Items {
		status := item.Status.State
		if status == "" {
			status = statusUnknown
		}

		if _, ok := counts[status]; !ok {
			counts[status] = 1
		} else {
			counts[status] = counts[status] + 1
		}
	}
	for _, status := range nimServiceStatusValues {
		if _, ok := counts[status]; !ok {
			nimServiceStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nimServiceStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNIMPipelineMetrics(nimPipelineList *appsv1alpha1.NIMPipelineList) {
	nimPipelineStatusMetric.WithLabelValues("all").Set(float64(len(nimPipelineList.Items)))
	counts := make(map[string]int)
	for _, item := range nimPipelineList.Items {
		status := item.Status.State
		if status == "" {
			status = statusUnknown
		}

		if _, ok := counts[status]; !ok {
			counts[status] = 1
		} else {
			counts[status] = counts[status] + 1
		}
	}
	for _, status := range nimPipelineStatusValues {
		if _, ok := counts[status]; !ok {
			nimPipelineStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nimPipelineStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refresNemoServiceMetrics(nemoServiceList *appsv1alpha1.NemoServiceList) {
	nemoServiceStatusMetric.WithLabelValues("all").Set(float64(len(nemoServiceList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoServiceList.Items {
		status := item.Status.State
		if status == "" {
			status = statusUnknown
		}

		if _, ok := counts[status]; !ok {
			counts[status] = 1
		} else {
			counts[status] = counts[status] + 1
		}
	}
	for _, status := range nemoServiceStatusValues {
		if _, ok := counts[status]; !ok {
			nemoServiceStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoServiceStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}
