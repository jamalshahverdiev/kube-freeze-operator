package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ActiveFreezePolicies tracks the number of currently active freeze policies by type
	ActiveFreezePolicies = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "freeze_operator_active_policies_total",
			Help: "Number of currently active freeze policies by type (maintenancewindow, changefreeze, freezeexception)",
		},
		[]string{"policy_type", "policy_name"},
	)

	// DeniedRequests tracks the number of denied admission requests
	DeniedRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "freeze_operator_denied_requests_total",
			Help: "Total number of denied admission requests by policy",
		},
		[]string{"policy_type", "policy_name", "namespace", "kind", "action"},
	)

	// AllowedRequests tracks the number of allowed admission requests
	AllowedRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "freeze_operator_allowed_requests_total",
			Help: "Total number of allowed admission requests",
		},
		[]string{"namespace", "kind", "action"},
	)

	// ExceptionOverrides tracks the number of times an exception overrode a deny policy
	ExceptionOverrides = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "freeze_operator_exception_overrides_total",
			Help: "Total number of times a FreezeException overrode a deny policy",
		},
		[]string{"exception_name", "policy_type", "policy_name"},
	)

	// ReconciliationDuration tracks controller reconciliation duration
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "freeze_operator_reconciliation_duration_seconds",
			Help:    "Duration of controller reconciliation in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	// CronJobSuspensions tracks the number of CronJobs currently suspended by policies
	CronJobSuspensions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "freeze_operator_cronjob_suspensions_total",
			Help: "Number of CronJobs currently suspended by freeze policies",
		},
		[]string{"policy_type", "policy_name", "namespace"},
	)

	// APIRequests tracks CI helper API evaluate requests by decision
	APIRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "freeze_operator_api_requests_total",
			Help: "Total number of CI helper API evaluate requests by decision",
		},
		[]string{"decision", "namespace", "kind", "action"},
	)

	// APILatency tracks CI helper API evaluate request duration
	APILatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "freeze_operator_api_latency_seconds",
			Help:    "Duration of CI helper API evaluate requests in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		},
	)

	// APIErrors tracks CI helper API errors (invalid requests, internal errors)
	APIErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "freeze_operator_api_errors_total",
			Help: "Total number of CI helper API errors by type",
		},
		[]string{"error_type"},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		ActiveFreezePolicies,
		DeniedRequests,
		AllowedRequests,
		ExceptionOverrides,
		ReconciliationDuration,
		CronJobSuspensions,
		APIRequests,
		APILatency,
		APIErrors,
	)
}
