package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BatchesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_b_batches_received_total",
			Help: "Total number of batches received",
		},
		[]string{"source"},
	)

	ItemsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_b_items_processed_total",
			Help: "Total number of items processed",
		},
		[]string{"status"},
	)

	WebhooksSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "system_b_webhooks_sent_total",
			Help: "Total number of webhooks sent",
		},
		[]string{"status"},
	)

	ItemsPending = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_b_items_pending_count",
			Help: "Number of items pending processing",
		},
	)

	ItemsProcessing = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "system_b_items_processing_count",
			Help: "Number of items currently processing",
		},
	)

	WorkerActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "system_b_worker_pool_active_count",
			Help: "Number of active workers",
		},
		[]string{"pool"},
	)

	ItemProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "system_b_item_processing_duration_seconds",
			Help:    "Duration of item processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)

	WebhookDeliveryDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "system_b_webhook_delivery_duration_seconds",
			Help:    "Duration of webhook delivery",
			Buckets: prometheus.DefBuckets,
		},
	)
)
