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

	nemoDatastoreStatusValues = []string{
		appsv1alpha1.NemoDatastoreStatusFailed,
		appsv1alpha1.NemoDatastoreStatusReady,
		appsv1alpha1.NemoDatastoreStatusNotReady,
		statusUnknown,
	}
	nemoDatastoreStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemo_datastore_status_total",
			Help: "Total number of Nemo Datastores with specific status value.",
		},
		[]string{"status"},
	)

	nemoEvaluatorStatusValues = []string{
		appsv1alpha1.NemoEvaluatorStatusFailed,
		appsv1alpha1.NemoEvaluatorStatusReady,
		appsv1alpha1.NemoEvaluatorStatusNotReady,
		statusUnknown,
	}
	nemoEvaluatorStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemo_evaluator_status_total",
			Help: "Total number of Nemo Evaluators with specific status value.",
		},
		[]string{"status"},
	)

	nemoEntitystoreStatusValues = []string{
		appsv1alpha1.NemoEntitystoreStatusFailed,
		appsv1alpha1.NemoEntitystoreStatusReady,
		appsv1alpha1.NemoEntitystoreStatusNotReady,
		statusUnknown,
	}
	nemoEntitystoreStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemo_entitystore_status_total",
			Help: "Total number of Nemo Entitystores with specific status value.",
		},
		[]string{"status"},
	)

	nemoCustomizerStatusValues = []string{
		appsv1alpha1.NemoCustomizerStatusFailed,
		appsv1alpha1.NemoCustomizerStatusReady,
		appsv1alpha1.NemoCustomizerStatusNotReady,
		statusUnknown,
	}
	nemoCustomizerStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemo_customizer_status_total",
			Help: "Total number of Nemo Customizers with specific status value.",
		},
		[]string{"status"},
	)

	nemoGuardrailStatusValues = []string{
		appsv1alpha1.NemoGuardrailStatusFailed,
		appsv1alpha1.NemoGuardrailStatusReady,
		appsv1alpha1.NemoGuardrailStatusNotReady,
		statusUnknown,
	}
	nemoGuardrailStatusMetric = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nemo_guardrail_status_total",
			Help: "Total number of Nemo Guardrails with specific status value.",
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

func refreshNemoDatastoreMetrics(nemoDatastoreList *appsv1alpha1.NemoDatastoreList) {
	nimPipelineStatusMetric.WithLabelValues("all").Set(float64(len(nemoDatastoreList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoDatastoreList.Items {
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
	for _, status := range nemoDatastoreStatusValues {
		if _, ok := counts[status]; !ok {
			nemoDatastoreStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoDatastoreStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNemoEvaluatorMetrics(nemoEvaluatorList *appsv1alpha1.NemoEvaluatorList) {
	nemoEvaluatorStatusMetric.WithLabelValues("all").Set(float64(len(nemoEvaluatorList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoEvaluatorList.Items {
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
	for _, status := range nemoEvaluatorStatusValues {
		if _, ok := counts[status]; !ok {
			nemoEvaluatorStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoEvaluatorStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNemoCustomizerMetrics(nemoCustomizerList *appsv1alpha1.NemoCustomizerList) {
	nemoCustomizerStatusMetric.WithLabelValues("all").Set(float64(len(nemoCustomizerList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoCustomizerList.Items {
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
	for _, status := range nemoCustomizerStatusValues {
		if _, ok := counts[status]; !ok {
			nemoCustomizerStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoCustomizerStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNemoEntitystoreMetrics(nemoEntitystoreList *appsv1alpha1.NemoEntitystoreList) {
	nemoEntitystoreStatusMetric.WithLabelValues("all").Set(float64(len(nemoEntitystoreList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoEntitystoreList.Items {
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
	for _, status := range nemoEntitystoreStatusValues {
		if _, ok := counts[status]; !ok {
			nemoEntitystoreStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoEntitystoreStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}

func refreshNemoGuardrailMetrics(nemoGuardrailList *appsv1alpha1.NemoGuardrailList) {
	nemoGuardrailStatusMetric.WithLabelValues("all").Set(float64(len(nemoGuardrailList.Items)))
	counts := make(map[string]int)
	for _, item := range nemoGuardrailList.Items {
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
	for _, status := range nemoGuardrailStatusValues {
		if _, ok := counts[status]; !ok {
			nemoGuardrailStatusMetric.WithLabelValues(status).Set(float64(0))
		}
	}
	for status, val := range counts {
		nemoGuardrailStatusMetric.WithLabelValues(status).Set(float64(val))
	}
}
