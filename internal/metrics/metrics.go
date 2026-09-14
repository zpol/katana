package metrics

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "katana"

// ClusterName is the Prometheus `cluster` label (KATANA_CLUSTER_NAME or CLUSTER_NAME).
func ClusterName() string {
	for _, k := range []string{"KATANA_CLUSTER_NAME", "CLUSTER_NAME"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return "unknown"
}

var (
	cluster = ClusterName()

	admissionRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "admission_requests_total",
		Help:      "Admission webhook reviews by Kubernetes result.",
	}, []string{"cluster", "result"})

	admissionOutcomes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "admission_outcomes_total",
		Help:      "Admission posture for fleet pies: compliant, blocked, dry_run_would_deny, unscanned_allowed, error.",
	}, []string{"cluster", "outcome"})

	admissionSeverity = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "admission_severity_total",
		Help:      "Highest image severity seen during admission evaluation.",
	}, []string{"cluster", "severity"})

	admissionPolicy = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "admission_policy_total",
		Help:      "Matched ImagePolicy during admission.",
	}, []string{"cluster", "policy", "action"})

	admissionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Name:        "admission_duration_seconds",
		Help:        "Admission review duration.",
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 12},
		ConstLabels: prometheus.Labels{"cluster": cluster},
	})

	xrayLookups = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "xray_lookups_total",
		Help:      "JFrog Xray artifact lookups by status.",
	}, []string{"cluster", "status"})

	xrayLookupDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Name:        "xray_lookup_duration_seconds",
		Help:        "JFrog Xray lookup duration.",
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 16, 32},
		ConstLabels: prometheus.Labels{"cluster": cluster},
	})

	collectorErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Name:        "cluster_collector_errors_total",
		Help:        "Failures listing cluster inventory (nodes / version).",
		ConstLabels: prometheus.Labels{"cluster": cluster},
	})
)

// MustRegister registers process metrics on r (typically prometheus.DefaultRegisterer).
func MustRegister(r prometheus.Registerer) {
	r.MustRegister(
		admissionRequests,
		admissionOutcomes,
		admissionSeverity,
		admissionPolicy,
		admissionDuration,
		xrayLookups,
		xrayLookupDuration,
		collectorErrors,
	)
}

// Handler serves Prometheus text exposition.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveAdmission records one webhook review.
func ObserveAdmission(result, outcome string, d time.Duration) {
	if result == "" {
		result = "error"
	}
	if outcome == "" {
		outcome = "error"
	}
	admissionRequests.WithLabelValues(cluster, result).Inc()
	admissionOutcomes.WithLabelValues(cluster, outcome).Inc()
	admissionDuration.Observe(d.Seconds())
}

// ObserveSeverity increments the severity counter.
func ObserveSeverity(sev string) {
	sev = strings.ToLower(strings.TrimSpace(sev))
	if sev == "" || sev == "unknown" {
		sev = "info"
	}
	admissionSeverity.WithLabelValues(cluster, sev).Inc()
}

// ObservePolicy increments the matched-policy counter.
func ObservePolicy(name, action string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "none"
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "allow"
	}
	admissionPolicy.WithLabelValues(cluster, name, action).Inc()
}

// ObserveXrayLookup records an Xray artifact summary call.
func ObserveXrayLookup(status string, d time.Duration) {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "indexed", "not_indexed", "unavailable":
	default:
		status = "unavailable"
	}
	xrayLookups.WithLabelValues(cluster, status).Inc()
	xrayLookupDuration.Observe(d.Seconds())
}

func incCollectorError() {
	collectorErrors.Inc()
}
