# ONUSLY Operational Intelligence: Service Level Objectives (SLOs) & Targets

## 1. Overview & Operational Principles
This document defines the quantitative Service Level Objectives (SLOs) and Service Level Indicators (SLIs) for the ONUSLY accountability platform.

All telemetry and observability signals adhere to the **Zero-Surveillance Privacy Contract**:
- No user passwords, session tokens, or private keys are ever stored or emitted in metrics/logs.
- No full URLs, browser histories, or query strings are retained.
- No keystrokes, active window titles, or screen captures are collected.
- Cardinality of metrics paths and labels is strictly bounded (UUIDs scrubbed to `:id`).

---

## 2. Core Operational SLOs

| SLO Code | Name | Target SLI | Measurement Window | Telemetry Source |
|---|---|---|---|---|
| **SLO-AVAIL-01** | **API Control Plane Availability** | **>= 99.9%** successful HTTP requests (non-5xx) | 30-day rolling | `onusly_api_requests_total`, `onusly_api_failures_total` |
| **SLO-LAT-01** | **Policy Sync Duration (P95)** | **< 500 ms** for authoritative blocklist generation | 7-day rolling | `onusly_api_latency_ms{path="/enforcement/blocklist"}` |
| **SLO-DNS-01** | **Local DNS Resolution Latency (P99)** | **< 10 ms** for cached queries, **< 50 ms** for forwarded | Continuous | Desktop agent internal DNS metrics |
| **SLO-HB-01** | **Heartbeat Telemetry Freshness** | **>= 99.5%** of active connected devices reporting within 5 mins | 24-hour rolling | `onusly_heartbeats_total` |
| **SLO-DEDUP-01** | **Lapse Notification Deduplication** | **100%** deduplication: zero repeated notifications per ongoing lapse | Continuous | `onusly_lapse_episodes_started_total` vs deliveries |
| **SLO-PRIV-01** | **Zero Surveillance & PII Compliance** | **100%** compliance (0 PII violations, 0 surveillance hooks) | Release gate | Security acceptance test suites (`test_d13`, `test_d14`) |

---

## 3. SLI Measurement Formulas

### 1. Control Plane Availability (`SLO-AVAIL-01`)
$$\text{Availability} = \frac{\sum \text{onusly\_api\_requests\_total} - \sum \text{onusly\_api\_failures\_total (5xx)}}{\sum \text{onusly\_api\_requests\_total}} \times 100\%$$

### 2. DNS Latency (`SLO-DNS-01`)
Measured locally by the stub resolver:
$$\text{Local Cache Hit Latency} \le 10\text{ ms}$$
$$\text{Forwarded Resolution Latency} \le 50\text{ ms}$$

### 3. Lapse Deduplication (`SLO-DEDUP-01`)
For each lapse episode identified by `lapseEpisodeId`:
$$\text{Notifications Sent per Ongoing Episode} = 1 \text{ (Owner)} + 1 \text{ (per Assigned Approver)}$$
Duplicate subsequent heartbeats reporting the same active lapse episode yield **0** additional notifications.

---

## 4. Operational Alerting Thresholds

- **P1 Alert (Critical)**:
  - API Control Plane error rate > 5% over 5-minute window.
  - Active device revocation cascade detected.
- **P2 Alert (Degraded)**:
  - Heartbeat failure rate > 2% over 15-minute window.
  - Policy sync latency P95 > 1000ms.
- **P3 Alert (Informational)**:
  - Single device DNS drift or policy stale state (> 24 hours).

