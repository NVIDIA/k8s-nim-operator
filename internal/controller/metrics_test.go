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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	io_prometheus_client "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Test nim operator metrics", func() {

	Context("Test status counters for nim cache", func() {
		It("Should pass", func() {
			nimCaches := &appsv1alpha1.NIMCacheList{
				Items: []appsv1alpha1.NIMCache{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache0",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMCacheStatus{
							State: appsv1alpha1.NimCacheStatusNotReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache1",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMCacheStatus{
							State: appsv1alpha1.NimCacheStatusReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache2",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMCacheStatus{},
					},
				},
			}

			refreshNIMCacheMetrics(nimCaches)
			g := nimCacheStatusMetric.WithLabelValues("all")
			metric := &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimCacheStatusMetric.WithLabelValues(statusUnknown)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimCacheStatusMetric.WithLabelValues(appsv1alpha1.NimCacheStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimCacheStatusMetric.WithLabelValues(appsv1alpha1.NimCacheStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			// Update metrics
			for i := range nimCaches.Items {
				nimCaches.Items[i].Status.State = appsv1alpha1.NimCacheStatusReady
			}
			refreshNIMCacheMetrics(nimCaches)
			g = nimCacheStatusMetric.WithLabelValues("all")
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimCacheStatusMetric.WithLabelValues(statusUnknown)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimCacheStatusMetric.WithLabelValues(appsv1alpha1.NimCacheStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimCacheStatusMetric.WithLabelValues(appsv1alpha1.NimCacheStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))
		})
	})

	Context("Test status counters for nim services", func() {
		It("Should pass", func() {
			nimServices := &appsv1alpha1.NIMServiceList{
				Items: []appsv1alpha1.NIMService{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache0",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMServiceStatus{
							State: appsv1alpha1.NIMServiceStatusNotReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache1",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMServiceStatus{
							State: appsv1alpha1.NIMServiceStatusReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache2",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMServiceStatus{
							State: appsv1alpha1.NIMServiceStatusFailed,
						},
					},
				},
			}

			refreshNIMServiceMetrics(nimServices)
			g := nimServiceStatusMetric.WithLabelValues("all")
			metric := &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusFailed)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			// Update metrics
			for i := range nimServices.Items {
				nimServices.Items[i].Status.State = appsv1alpha1.NIMServiceStatusFailed
			}
			refreshNIMServiceMetrics(nimServices)
			g = nimServiceStatusMetric.WithLabelValues("all")
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimServiceStatusMetric.WithLabelValues(statusUnknown)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimServiceStatusMetric.WithLabelValues(appsv1alpha1.NIMServiceStatusFailed)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))
		})
	})

	Context("Test status counters for nim pipelines", func() {
		It("Should pass", func() {
			nimPipelines := &appsv1alpha1.NIMPipelineList{
				Items: []appsv1alpha1.NIMPipeline{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache0",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMPipelineStatus{
							State: appsv1alpha1.NIMPipelineStatusReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache1",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMPipelineStatus{
							State: appsv1alpha1.NIMPipelineStatusNotReady,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-nimcache2",
							Namespace: "default",
						},
						Status: appsv1alpha1.NIMPipelineStatus{
							State: appsv1alpha1.NIMPipelineStatusFailed,
						},
					},
				},
			}

			refreshNIMPipelineMetrics(nimPipelines)
			g := nimPipelineStatusMetric.WithLabelValues("all")
			metric := &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusFailed)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)))

			// Update metrics
			for i := range nimPipelines.Items {
				nimPipelines.Items[i].Status.State = appsv1alpha1.NIMPipelineStatusNotReady
			}
			refreshNIMPipelineMetrics(nimPipelines)
			g = nimPipelineStatusMetric.WithLabelValues("all")
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimPipelineStatusMetric.WithLabelValues(statusUnknown)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusNotReady)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(3)))

			g = nimPipelineStatusMetric.WithLabelValues(appsv1alpha1.NIMPipelineStatusFailed)
			metric = &io_prometheus_client.Metric{}
			Expect(g.Write(metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(0)))
		})
	})
})
