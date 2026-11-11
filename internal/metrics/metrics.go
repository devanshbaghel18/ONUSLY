package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry coordinates privacy-safe metrics collection across all backend subsystems.
type Registry struct {
	mu sync.RWMutex

	// Counters
	requestsTotal        map[string]*atomic.Uint64 // method:path:status -> count
	failuresTotal        map[string]*atomic.Uint64 // method:path:status -> count
	dbOperationsTotal    map[string]*atomic.Uint64 // op:table:status -> count
	syncOutcomesTotal    map[string]*atomic.Uint64 // outcome -> count
	heartbeatsTotal      map[string]*atomic.Uint64 // platform:clientType:status -> count
	idempotencyHits      map[string]*atomic.Uint64 // path -> count
	idempotencyConflicts map[string]*atomic.Uint64 // path -> count
	conditionalConflicts map[string]*atomic.Uint64 // entity:op -> count

	// D14 Expanded Reliability Counters
	authFailuresTotal                 map[string]*atomic.Uint64 // reason -> count
	authzFailuresTotal                map[string]*atomic.Uint64 // resource -> count
	lapseEpisodesStarted              atomic.Uint64
	lapseEpisodesResolved             atomic.Uint64
	pairingAttemptsTotal              atomic.Uint64
	pairingSuccessTotal               atomic.Uint64
	pairingFailuresTotal              map[string]*atomic.Uint64 // reason -> count
	notificationDeliveriesTotal       map[string]*atomic.Uint64 // type -> count
	notificationDeliveryFailuresTotal map[string]*atomic.Uint64 // type -> count
	updateVerificationsTotal          map[string]*atomic.Uint64 // platform -> count
	updateVerificationFailuresTotal   map[string]*atomic.Uint64 // reason -> count

	policyVersionChanges atomic.Uint64

	// Latency Tracking (averages & counts in nanoseconds)
	apiLatencySum   map[string]*atomic.Uint64
	apiLatencyCount map[string]*atomic.Uint64

	dbLatencySum   map[string]*atomic.Uint64
	dbLatencyCount map[string]*atomic.Uint64

	policyGenLatencySum   atomic.Uint64
	policyGenLatencyCount atomic.Uint64
}

// Global instance for backend instrumentation
var DefaultRegistry = NewRegistry()

// NewRegistry initializes an empty metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		requestsTotal:                     make(map[string]*atomic.Uint64),
		failuresTotal:                     make(map[string]*atomic.Uint64),
		dbOperationsTotal:                 make(map[string]*atomic.Uint64),
		syncOutcomesTotal:                 make(map[string]*atomic.Uint64),
		heartbeatsTotal:                   make(map[string]*atomic.Uint64),
		idempotencyHits:                   make(map[string]*atomic.Uint64),
		idempotencyConflicts:             make(map[string]*atomic.Uint64),
		conditionalConflicts:             make(map[string]*atomic.Uint64),
		authFailuresTotal:                 make(map[string]*atomic.Uint64),
		authzFailuresTotal:                make(map[string]*atomic.Uint64),
		pairingFailuresTotal:              make(map[string]*atomic.Uint64),
		notificationDeliveriesTotal:       make(map[string]*atomic.Uint64),
		notificationDeliveryFailuresTotal: make(map[string]*atomic.Uint64),
		updateVerificationsTotal:          make(map[string]*atomic.Uint64),
		updateVerificationFailuresTotal:   make(map[string]*atomic.Uint64),
		apiLatencySum:                     make(map[string]*atomic.Uint64),
		apiLatencyCount:                   make(map[string]*atomic.Uint64),
		dbLatencySum:                      make(map[string]*atomic.Uint64),
		dbLatencyCount:                    make(map[string]*atomic.Uint64),
	}
}

// SanitizePath normalizes URL paths to prevent cardinality explosion and scrub IDs.
func SanitizePath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	sanitized := make([]string, 0, len(parts))

	for i, p := range parts {
		if p == "" {
			continue
		}
		// If segment looks like a UUID or ID (length >= 8 with hex/hyphen/digits), scrub it
		if (len(p) == 36 && strings.Count(p, "-") == 4) || (len(p) >= 12 && i > 0) {
			sanitized = append(sanitized, ":id")
		} else {
			sanitized = append(sanitized, p)
		}
	}
	if len(sanitized) == 0 {
		return "/"
	}
	return "/" + strings.Join(sanitized, "/")
}

// RecordHTTPRequest tracks an API request's outcome and duration in a privacy-safe manner.
func (r *Registry) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	normPath := SanitizePath(path)
	normMethod := strings.ToUpper(method)
	statusStr := http.StatusText(statusCode)
	if statusStr == "" {
		statusStr = "Unknown"
	}

	key := normMethod + " " + normPath + " (" + statusStr + ")"

	r.mu.Lock()
	counter, ok := r.requestsTotal[key]
	if !ok {
		counter = &atomic.Uint64{}
		r.requestsTotal[key] = counter
	}
	if statusCode >= 400 {
		fCounter, fOk := r.failuresTotal[key]
		if !fOk {
			fCounter = &atomic.Uint64{}
			r.failuresTotal[key] = fCounter
		}
		fCounter.Add(1)
	}

	latKey := normMethod + " " + normPath
	sum, sOk := r.apiLatencySum[latKey]
	if !sOk {
		sum = &atomic.Uint64{}
		r.apiLatencySum[latKey] = sum
	}
	cnt, cOk := r.apiLatencyCount[latKey]
	if !cOk {
		cnt = &atomic.Uint64{}
		r.apiLatencyCount[latKey] = cnt
	}
	r.mu.Unlock()

	counter.Add(1)
	sum.Add(uint64(duration.Nanoseconds()))
	cnt.Add(1)
}

// RecordDBOperation records a DynamoDB operation, its outcome, and latency.
func (r *Registry) RecordDBOperation(op, table string, success bool, duration time.Duration) {
	status := "success"
	if !success {
		status = "error"
	}
	key := op + ":" + table + ":" + status

	r.mu.Lock()
	counter, ok := r.dbOperationsTotal[key]
	if !ok {
		counter = &atomic.Uint64{}
		r.dbOperationsTotal[key] = counter
	}

	latKey := op + ":" + table
	sum, sOk := r.dbLatencySum[latKey]
	if !sOk {
		sum = &atomic.Uint64{}
		r.dbLatencySum[latKey] = sum
	}
	cnt, cOk := r.dbLatencyCount[latKey]
	if !cOk {
		cnt = &atomic.Uint64{}
		r.dbLatencyCount[latKey] = cnt
	}
	r.mu.Unlock()

	counter.Add(1)
	sum.Add(uint64(duration.Nanoseconds()))
	cnt.Add(1)
}

// RecordPolicyGeneration records the duration taken to generate an authoritative blocklist.
func (r *Registry) RecordPolicyGeneration(duration time.Duration) {
	r.policyGenLatencySum.Add(uint64(duration.Nanoseconds()))
	r.policyGenLatencyCount.Add(1)
}

// RecordPolicyVersionChange increments the policy version mutation counter.
func (r *Registry) RecordPolicyVersionChange() {
	r.policyVersionChanges.Add(1)
}

// RecordSyncOutcome records whether a policy sync resulted in fresh content, 304 cached, or rejection.
func (r *Registry) RecordSyncOutcome(outcome string) {
	r.mu.Lock()
	counter, ok := r.syncOutcomesTotal[outcome]
	if !ok {
		counter = &atomic.Uint64{}
		r.syncOutcomesTotal[outcome] = counter
	}
	r.mu.Unlock()
	counter.Add(1)
}

// RecordHeartbeat records an incoming device heartbeat status without logging any device or user IDs.
func (r *Registry) RecordHeartbeat(platform, clientType, status string) {
	key := strings.ToLower(platform) + ":" + strings.ToLower(clientType) + ":" + strings.ToLower(status)
	r.mu.Lock()
	counter, ok := r.heartbeatsTotal[key]
	if !ok {
		counter = &atomic.Uint64{}
		r.heartbeatsTotal[key] = counter
	}
	r.mu.Unlock()
	counter.Add(1)
}

// RecordIdempotencyHit records a successful idempotent replay.
func (r *Registry) RecordIdempotencyHit(path string) {
	norm := SanitizePath(path)
	r.mu.Lock()
	c, ok := r.idempotencyHits[norm]
	if !ok {
		c = &atomic.Uint64{}
		r.idempotencyHits[norm] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// RecordIdempotencyConflict records an idempotency key conflict (in-flight or payload mismatch).
func (r *Registry) RecordIdempotencyConflict(path string) {
	norm := SanitizePath(path)
	r.mu.Lock()
	c, ok := r.idempotencyConflicts[norm]
	if !ok {
		c = &atomic.Uint64{}
		r.idempotencyConflicts[norm] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// RecordConditionalConflict records an optimistic lock or DynamoDB conditional check conflict.
func (r *Registry) RecordConditionalConflict(entity, op string) {
	key := entity + ":" + op
	r.mu.Lock()
	c, ok := r.conditionalConflicts[key]
	if !ok {
		c = &atomic.Uint64{}
		r.conditionalConflicts[key] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// RecordAuthFailure records an authentication rejection with a bounded reason category.
func (r *Registry) RecordAuthFailure(reason string) {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		reason = "unknown"
	}
	r.mu.Lock()
	c, ok := r.authFailuresTotal[reason]
	if !ok {
		c = &atomic.Uint64{}
		r.authFailuresTotal[reason] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// RecordAuthzFailure records an authorization rejection by resource type.
func (r *Registry) RecordAuthzFailure(resource string) {
	resource = strings.ToLower(strings.TrimSpace(resource))
	if resource == "" {
		resource = "unknown"
	}
	r.mu.Lock()
	c, ok := r.authzFailuresTotal[resource]
	if !ok {
		c = &atomic.Uint64{}
		r.authzFailuresTotal[resource] = c
	}
	r.mu.Unlock()
	c.Add(1)
}

// RecordLapseEpisode tracks state transitions into and out of enforcement lapse episodes.
func (r *Registry) RecordLapseEpisode(action string) {
	switch strings.ToLower(action) {
	case "started", "start":
		r.lapseEpisodesStarted.Add(1)
	case "resolved", "resolve":
		r.lapseEpisodesResolved.Add(1)
	}
}

// RecordPairingOutcome tracks pairing attempts, successes, and categorized failures.
func (r *Registry) RecordPairingOutcome(outcome, reason string) {
	switch strings.ToLower(outcome) {
	case "attempt":
		r.pairingAttemptsTotal.Add(1)
	case "success":
		r.pairingSuccessTotal.Add(1)
	case "failure":
		reason = strings.ToLower(strings.TrimSpace(reason))
		if reason == "" {
			reason = "unknown"
		}
		r.mu.Lock()
		c, ok := r.pairingFailuresTotal[reason]
		if !ok {
			c = &atomic.Uint64{}
			r.pairingFailuresTotal[reason] = c
		}
		r.mu.Unlock()
		c.Add(1)
	}
}

// RecordNotificationDelivery tracks notifications successfully sent and failures.
func (r *Registry) RecordNotificationDelivery(notifType string, success bool) {
	notifType = strings.ToLower(strings.TrimSpace(notifType))
	if notifType == "" {
		notifType = "unknown"
	}
	r.mu.Lock()
	if success {
		c, ok := r.notificationDeliveriesTotal[notifType]
		if !ok {
			c = &atomic.Uint64{}
			r.notificationDeliveriesTotal[notifType] = c
		}
		r.mu.Unlock()
		c.Add(1)
	} else {
		c, ok := r.notificationDeliveryFailuresTotal[notifType]
		if !ok {
			c = &atomic.Uint64{}
			r.notificationDeliveryFailuresTotal[notifType] = c
		}
		r.mu.Unlock()
		c.Add(1)
	}
}

// RecordUpdateVerification tracks release update verification checks.
func (r *Registry) RecordUpdateVerification(platform string, success bool, failureReason string) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		platform = "unknown"
	}
	r.mu.Lock()
	c, ok := r.updateVerificationsTotal[platform]
	if !ok {
		c = &atomic.Uint64{}
		r.updateVerificationsTotal[platform] = c
	}
	if !success {
		failureReason = strings.ToLower(strings.TrimSpace(failureReason))
		if failureReason == "" {
			failureReason = "verification_failed"
		}
		fc, fOk := r.updateVerificationFailuresTotal[failureReason]
		if !fOk {
			fc = &atomic.Uint64{}
			r.updateVerificationFailuresTotal[failureReason] = fc
		}
		fc.Add(1)
	}
	r.mu.Unlock()
	c.Add(1)
}

// Snapshot returns a JSON-serializable map of all metrics.
func (r *Registry) Snapshot() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reqs := make(map[string]uint64, len(r.requestsTotal))
	for k, v := range r.requestsTotal {
		reqs[k] = v.Load()
	}

	fails := make(map[string]uint64, len(r.failuresTotal))
	for k, v := range r.failuresTotal {
		fails[k] = v.Load()
	}

	dbOps := make(map[string]uint64, len(r.dbOperationsTotal))
	for k, v := range r.dbOperationsTotal {
		dbOps[k] = v.Load()
	}

	syncs := make(map[string]uint64, len(r.syncOutcomesTotal))
	for k, v := range r.syncOutcomesTotal {
		syncs[k] = v.Load()
	}

	hbs := make(map[string]uint64, len(r.heartbeatsTotal))
	for k, v := range r.heartbeatsTotal {
		hbs[k] = v.Load()
	}

	idempHits := make(map[string]uint64, len(r.idempotencyHits))
	for k, v := range r.idempotencyHits {
		idempHits[k] = v.Load()
	}

	idempConflicts := make(map[string]uint64, len(r.idempotencyConflicts))
	for k, v := range r.idempotencyConflicts {
		idempConflicts[k] = v.Load()
	}

	condConflicts := make(map[string]uint64, len(r.conditionalConflicts))
	for k, v := range r.conditionalConflicts {
		condConflicts[k] = v.Load()
	}

	authFails := make(map[string]uint64, len(r.authFailuresTotal))
	for k, v := range r.authFailuresTotal {
		authFails[k] = v.Load()
	}

	authzFails := make(map[string]uint64, len(r.authzFailuresTotal))
	for k, v := range r.authzFailuresTotal {
		authzFails[k] = v.Load()
	}

	pairingFails := make(map[string]uint64, len(r.pairingFailuresTotal))
	for k, v := range r.pairingFailuresTotal {
		pairingFails[k] = v.Load()
	}

	notifDeliv := make(map[string]uint64, len(r.notificationDeliveriesTotal))
	for k, v := range r.notificationDeliveriesTotal {
		notifDeliv[k] = v.Load()
	}

	notifFails := make(map[string]uint64, len(r.notificationDeliveryFailuresTotal))
	for k, v := range r.notificationDeliveryFailuresTotal {
		notifFails[k] = v.Load()
	}

	updateVerifs := make(map[string]uint64, len(r.updateVerificationsTotal))
	for k, v := range r.updateVerificationsTotal {
		updateVerifs[k] = v.Load()
	}

	updateFails := make(map[string]uint64, len(r.updateVerificationFailuresTotal))
	for k, v := range r.updateVerificationFailuresTotal {
		updateFails[k] = v.Load()
	}

	apiLatencies := make(map[string]float64, len(r.apiLatencySum))
	for k, sum := range r.apiLatencySum {
		cnt := r.apiLatencyCount[k].Load()
		if cnt > 0 {
			apiLatencies[k] = float64(sum.Load()) / float64(cnt) / 1e6
		}
	}

	dbLatencies := make(map[string]float64, len(r.dbLatencySum))
	for k, sum := range r.dbLatencySum {
		cnt := r.dbLatencyCount[k].Load()
		if cnt > 0 {
			dbLatencies[k] = float64(sum.Load()) / float64(cnt) / 1e6
		}
	}

	var avgPolicyGenMs float64
	pgCnt := r.policyGenLatencyCount.Load()
	if pgCnt > 0 {
		avgPolicyGenMs = float64(r.policyGenLatencySum.Load()) / float64(pgCnt) / 1e6
	}

	return map[string]interface{}{
		"api_requests_total":                   reqs,
		"api_failures_total":                   fails,
		"api_latency_avg_ms":                   apiLatencies,
		"db_operations_total":                  dbOps,
		"db_latency_avg_ms":                    dbLatencies,
		"policy_version_changes":               r.policyVersionChanges.Load(),
		"policy_generation_avg_ms":             avgPolicyGenMs,
		"policy_sync_outcomes":                 syncs,
		"heartbeats_total":                     hbs,
		"idempotency_hits":                     idempHits,
		"idempotency_conflicts":                idempConflicts,
		"conditional_write_conflicts":          condConflicts,
		"auth_failures_total":                  authFails,
		"authz_failures_total":                 authzFails,
		"lapse_episodes_started_total":         r.lapseEpisodesStarted.Load(),
		"lapse_episodes_resolved_total":        r.lapseEpisodesResolved.Load(),
		"pairing_attempts_total":               r.pairingAttemptsTotal.Load(),
		"pairing_success_total":                r.pairingSuccessTotal.Load(),
		"pairing_failures_total":               pairingFails,
		"notification_deliveries_total":        notifDeliv,
		"notification_delivery_failures_total": notifFails,
		"update_verifications_total":           updateVerifs,
		"update_verification_failures_total":   updateFails,
		"privacy_guarantee":                    "Zero PII: no tokens, passwords, proofs, DNS domains, or process names recorded.",
	}
}

// PrometheusFormat formats the current registry metrics in the Prometheus text exposition standard.
func (r *Registry) PrometheusFormat() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// Helper to write help and type headers
	writeHeader := func(name, help, mType string) {
		sb.WriteString(fmt.Sprintf("# HELP %s %s\n# TYPE %s %s\n", name, help, name, mType))
	}

	// 1. API Requests Total
	if len(r.requestsTotal) > 0 {
		writeHeader("onusly_api_requests_total", "Total count of HTTP API requests", "counter")
		keys := sortedKeys(r.requestsTotal)
		for _, k := range keys {
			// Format is "METHOD PATH (STATUS)"
			method, path, status := parseRequestKey(k)
			sb.WriteString(fmt.Sprintf("onusly_api_requests_total{method=%q,path=%q,status=%q} %d\n", method, path, status, r.requestsTotal[k].Load()))
		}
	}

	// 2. API Failures Total
	if len(r.failuresTotal) > 0 {
		writeHeader("onusly_api_failures_total", "Total count of failed HTTP API requests (>= 400)", "counter")
		keys := sortedKeys(r.failuresTotal)
		for _, k := range keys {
			method, path, status := parseRequestKey(k)
			sb.WriteString(fmt.Sprintf("onusly_api_failures_total{method=%q,path=%q,status=%q} %d\n", method, path, status, r.failuresTotal[k].Load()))
		}
	}

	// 3. Auth and Authz Failures
	writeHeader("onusly_auth_failures_total", "Total authentication failures by reason", "counter")
	if len(r.authFailuresTotal) == 0 {
		sb.WriteString("onusly_auth_failures_total{reason=\"none\"} 0\n")
	} else {
		for _, k := range sortedKeys(r.authFailuresTotal) {
			sb.WriteString(fmt.Sprintf("onusly_auth_failures_total{reason=%q} %d\n", k, r.authFailuresTotal[k].Load()))
		}
	}

	writeHeader("onusly_authz_failures_total", "Total authorization failures by resource", "counter")
	if len(r.authzFailuresTotal) == 0 {
		sb.WriteString("onusly_authz_failures_total{resource=\"none\"} 0\n")
	} else {
		for _, k := range sortedKeys(r.authzFailuresTotal) {
			sb.WriteString(fmt.Sprintf("onusly_authz_failures_total{resource=%q} %d\n", k, r.authzFailuresTotal[k].Load()))
		}
	}

	// 4. Lapse Episodes
	writeHeader("onusly_lapse_episodes_started_total", "Total enforcement lapse episodes started", "counter")
	sb.WriteString(fmt.Sprintf("onusly_lapse_episodes_started_total %d\n", r.lapseEpisodesStarted.Load()))

	writeHeader("onusly_lapse_episodes_resolved_total", "Total enforcement lapse episodes resolved", "counter")
	sb.WriteString(fmt.Sprintf("onusly_lapse_episodes_resolved_total %d\n", r.lapseEpisodesResolved.Load()))

	// 5. Pairing Outcomes
	writeHeader("onusly_pairing_attempts_total", "Total device pairing requests initiated", "counter")
	sb.WriteString(fmt.Sprintf("onusly_pairing_attempts_total %d\n", r.pairingAttemptsTotal.Load()))

	writeHeader("onusly_pairing_success_total", "Total successful device pairings confirmed", "counter")
	sb.WriteString(fmt.Sprintf("onusly_pairing_success_total %d\n", r.pairingSuccessTotal.Load()))

	if len(r.pairingFailuresTotal) > 0 {
		writeHeader("onusly_pairing_failures_total", "Total failed device pairings by reason", "counter")
		for _, k := range sortedKeys(r.pairingFailuresTotal) {
			sb.WriteString(fmt.Sprintf("onusly_pairing_failures_total{reason=%q} %d\n", k, r.pairingFailuresTotal[k].Load()))
		}
	}

	// 6. Database Operations Total
	if len(r.dbOperationsTotal) > 0 {
		writeHeader("onusly_db_operations_total", "Total DynamoDB operations by op, table, and status", "counter")
		for _, k := range sortedKeys(r.dbOperationsTotal) {
			parts := strings.Split(k, ":")
			op, table, status := "unknown", "unknown", "unknown"
			if len(parts) >= 3 {
				op, table, status = parts[0], parts[1], parts[2]
			}
			sb.WriteString(fmt.Sprintf("onusly_db_operations_total{op=%q,table=%q,status=%q} %d\n", op, table, status, r.dbOperationsTotal[k].Load()))
		}
	}

	// 7. Policy Sync Outcomes
	if len(r.syncOutcomesTotal) > 0 {
		writeHeader("onusly_sync_outcomes_total", "Total policy sync outcomes by outcome type", "counter")
		for _, k := range sortedKeys(r.syncOutcomesTotal) {
			sb.WriteString(fmt.Sprintf("onusly_sync_outcomes_total{outcome=%q} %d\n", k, r.syncOutcomesTotal[k].Load()))
		}
	}

	// 8. Heartbeats Total
	if len(r.heartbeatsTotal) > 0 {
		writeHeader("onusly_heartbeats_total", "Total device heartbeats received", "counter")
		for _, k := range sortedKeys(r.heartbeatsTotal) {
			parts := strings.Split(k, ":")
			plat, cType, status := "unknown", "unknown", "unknown"
			if len(parts) >= 3 {
				plat, cType, status = parts[0], parts[1], parts[2]
			}
			sb.WriteString(fmt.Sprintf("onusly_heartbeats_total{platform=%q,client_type=%q,status=%q} %d\n", plat, cType, status, r.heartbeatsTotal[k].Load()))
		}
	}

	// 9. Notifications Total
	if len(r.notificationDeliveriesTotal) > 0 {
		writeHeader("onusly_notifications_total", "Total notifications delivered by type", "counter")
		for _, k := range sortedKeys(r.notificationDeliveriesTotal) {
			sb.WriteString(fmt.Sprintf("onusly_notifications_total{type=%q} %d\n", k, r.notificationDeliveriesTotal[k].Load()))
		}
	}

	// 10. Update Verifications
	if len(r.updateVerificationsTotal) > 0 {
		writeHeader("onusly_update_verifications_total", "Total update verifications performed by platform", "counter")
		for _, k := range sortedKeys(r.updateVerificationsTotal) {
			sb.WriteString(fmt.Sprintf("onusly_update_verifications_total{platform=%q} %d\n", k, r.updateVerificationsTotal[k].Load()))
		}
	}

	// 11. Policy Version Changes
	writeHeader("onusly_policy_version_changes_total", "Total policy version updates", "counter")
	sb.WriteString(fmt.Sprintf("onusly_policy_version_changes_total %d\n", r.policyVersionChanges.Load()))

	// 12. Latencies (Gauges)
	if len(r.apiLatencySum) > 0 {
		writeHeader("onusly_api_latency_ms", "Average HTTP API latency in milliseconds", "gauge")
		for _, k := range sortedKeys(r.apiLatencySum) {
			cnt := r.apiLatencyCount[k].Load()
			if cnt > 0 {
				parts := strings.SplitN(k, " ", 2)
				method := "GET"
				path := k
				if len(parts) == 2 {
					method = parts[0]
					path = parts[1]
				}
				avgMs := float64(r.apiLatencySum[k].Load()) / float64(cnt) / 1e6
				sb.WriteString(fmt.Sprintf("onusly_api_latency_ms{method=%q,path=%q} %.3f\n", method, path, avgMs))
			}
		}
	}

	return sb.String()
}

func parseRequestKey(k string) (method, path, status string) {
	method = "UNKNOWN"
	path = "/"
	status = "OK"

	parts := strings.SplitN(k, " ", 3)
	if len(parts) >= 1 {
		method = parts[0]
	}
	if len(parts) >= 2 {
		path = parts[1]
	}
	if len(parts) >= 3 {
		status = strings.Trim(parts[2], "()")
	}
	return
}

func sortedKeys(m map[string]*atomic.Uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isAuthorizedMetricsRequest verifies whether the caller is permitted to read operational metrics.
func isAuthorizedMetricsRequest(req *http.Request) bool {
	// If internal-only restriction is enabled in config/environment
	tokenReq := os.Getenv("ONUSLY_METRICS_TOKEN")
	if tokenReq != "" {
		clientToken := req.Header.Get("X-Internal-Token")
		if clientToken == tokenReq {
			return true
		}
	}

	// By default allow loopback callers (internal monitoring / Prometheus scraper / localhost)
	host := req.RemoteAddr
	if colon := strings.LastIndex(host, ":"); colon != -1 {
		host = host[:colon]
	}
	host = strings.Trim(host, "[]")
	if host == "127.0.0.1" || host == "::1" || host == "localhost" || host == "" {
		return true
	}

	// If no internal token configured and running locally, allow
	if tokenReq == "" {
		return true
	}

	return false
}

// Handler returns an http.HandlerFunc that outputs metrics as Prometheus or JSON.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if !isAuthorizedMetricsRequest(req) {
			http.Error(w, `{"error":"unauthorized access to internal metrics"}`, http.StatusForbidden)
			return
		}

		format := req.URL.Query().Get("format")
		accept := req.Header.Get("Accept")

		if strings.ToLower(format) == "prometheus" ||
			strings.Contains(accept, "text/plain") ||
			strings.Contains(accept, "application/openmetrics-text") {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(r.PrometheusFormat()))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(r.Snapshot())
	}
}

