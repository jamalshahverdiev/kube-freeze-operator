# API Reference

Complete API reference for kube-freeze-operator CRDs.

## API Versions

- **v1alpha1**: Current API version
- **API Group**: `freeze-operator.io`

All CRDs are **Cluster-scoped**.

---

## MaintenanceWindow

**Kind:** MaintenanceWindow
**Scope:** Cluster

Defines recurring time windows when changes are allowed. Outside of windows, matching actions are denied.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timezone` | string | Yes | IANA timezone name (e.g., `"UTC"`, `"America/New_York"`) |
| `mode` | string | Yes | Window evaluation mode. Currently only `DenyOutsideWindows` |
| `windows` | [][WindowSpec](#windowspec) | Yes | One or more recurring maintenance intervals (min 1) |
| `target` | [TargetSpec](#targetspec) | Yes | Namespaces, objects, and kinds this policy applies to |
| `rules` | [PolicyRulesSpec](#policyrulesspec) | Yes | Actions denied outside windows |
| `behavior` | [PolicyBehaviorSpec](#policybehaviorspec) | No | Side-effects (CronJob suspension, GitOps pause) |
| `message` | [MessageSpec](#messagespec) | No | Custom denial message |

### WindowSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Human-readable identifier (min 1 char) |
| `schedule` | string | Yes | 5-field cron expression (e.g., `"0 1 * * *"`) |
| `duration` | duration | Yes | Window length (e.g., `"6h"`, `"30m"`) |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | bool | Whether a window is currently open |
| `activeWindow` | WindowStatus | Currently open window (name, startTime, endTime) |
| `nextWindow` | WindowStatus | Next upcoming window |
| `observedGeneration` | int64 | Last observed spec generation |
| `gitopsPausedCount` | int | Number of GitOps resources paused |
| `conditions` | []metav1.Condition | Standard conditions |

---

## ChangeFreeze

**Kind:** ChangeFreeze
**Scope:** Cluster

Blocks changes during a fixed time period.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `startTime` | metav1.Time | Yes | RFC3339 timestamp when freeze begins |
| `endTime` | metav1.Time | Yes | RFC3339 timestamp when freeze ends. Must be after `startTime` |
| `timezone` | *string | No | IANA timezone name (for display/UX) |
| `target` | [TargetSpec](#targetspec) | Yes | Namespaces, objects, and kinds this policy applies to |
| `rules` | [PolicyRulesSpec](#policyrulesspec) | Yes | Actions denied during [startTime, endTime] |
| `behavior` | [PolicyBehaviorSpec](#policybehaviorspec) | No | Side-effects (CronJob suspension, GitOps pause) |
| `message` | [MessageSpec](#messagespec) | No | Custom denial message |

**Validation:** `endTime` must be after `startTime` (enforced by CEL rule).

### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | bool | Whether the freeze is currently active |
| `timeRemaining` | *metav1.Duration | Time until freeze ends |
| `observedGeneration` | int64 | Last observed spec generation |
| `gitopsPausedCount` | int | Number of GitOps resources paused |
| `conditions` | []metav1.Condition | Standard conditions |

---

## FreezeException

**Kind:** FreezeException
**Scope:** Cluster

Overrides freeze policies to allow specific changes during a freeze period.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `activeFrom` | metav1.Time | Yes | When this exception becomes effective |
| `activeTo` | metav1.Time | Yes | When this exception expires. Must be after `activeFrom` |
| `target` | [TargetSpec](#targetspec) | Yes | Namespaces, objects, and kinds this exception applies to |
| `allow` | []Action | Yes | Actions allowed despite freeze (min 1) |
| `reason` | string | Yes | Why this exception exists (min 1 char) |
| `ticketURL` | string | No | Link to approval/tracking ticket |
| `approvedBy` | string | No | Free-form approver identifier |
| `constraints` | [ConstraintsSpec](#constraintsspec) | No | Optional limits on exception usage |

**Validation:** `activeTo` must be after `activeFrom` (enforced by CEL rule).

### ConstraintsSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `requireLabels` | map[string]string | No | Labels that must exist on the target object |
| `allowedUsers` | []string | No | Restrict to these usernames |
| `allowedGroups` | []string | No | Restrict to these groups |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | bool | Whether the exception is currently active |
| `observedGeneration` | int64 | Last observed spec generation |
| `conditions` | []metav1.Condition | Standard conditions |

---

## Common Types

### Action

Enum: `CREATE`, `DELETE`, `ROLL_OUT`, `SCALE`

| Action | Description |
|--------|-------------|
| `CREATE` | Creating new resources |
| `DELETE` | Deleting resources |
| `ROLL_OUT` | Changes to `spec.template` (image, env, etc.) |
| `SCALE` | Changes to `spec.replicas` |

### TargetKind

Enum: `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`

### TargetSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespaceSelector` | *metav1.LabelSelector | No | Select namespaces by labels |
| `objectSelector` | *metav1.LabelSelector | No | Select objects by labels |
| `kinds` | []TargetKind | Yes | Resource kinds (min 1) |

### PolicyRulesSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `deny` | []Action | Yes | Actions denied when policy is active (min 1) |

### MessageSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `reason` | string | No | Short human-readable description |
| `docsURL` | string | No | Link to documentation |
| `contact` | string | No | Contact point (team, oncall, etc.) |

### PolicyBehaviorSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `suspendCronJobs` | bool | No | Suspend matching CronJobs while policy is active |
| `gitops` | [GitOpsSpec](#gitopsspec) | No | GitOps pause/resume configuration |

### GitOpsSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `enabled` | bool | Yes | Activate GitOps integration |
| `providers` | []string | No | Engines to manage: `"argocd"`, `"flux"` |
| `argocd` | GitOpsArgoCDSpec | No | ArgoCD-specific configuration |
| `flux` | GitOpsFluxSpec | No | Flux-specific configuration |

---

## Priority and Conflict Resolution

1. **FreezeException** (highest) — allows changes
2. **ChangeFreeze** — denies changes
3. **MaintenanceWindow** (outside window) — denies changes
4. **No match** — default allow

When multiple deny policies match, the one with the earliest `nextAllowedTime` is selected.

---

## Annotations

| Annotation | Added To | Description |
|------------|----------|-------------|
| `freeze-operator.io/managed-by` | CronJob | Policy name managing this CronJob |
| `freeze-operator.io/original-suspend` | CronJob | Original suspend state before operator modified it |

---

## Metrics

Metrics are exposed on `:8443` (TLS + authn/authz via controller-runtime).

### Controller Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `freeze_operator_active_policies_total` | Gauge | Active policies by type |
| `freeze_operator_denied_requests_total` | Counter | Admission requests denied |
| `freeze_operator_allowed_requests_total` | Counter | Admission requests allowed |
| `freeze_operator_exception_overrides_total` | Counter | Exception overrides applied |
| `freeze_operator_reconciliation_duration_seconds` | Histogram | Reconciliation duration |
| `freeze_operator_cronjob_suspensions_total` | Counter | CronJob suspend/resume operations |

### CI Helper API Metrics (v3.0+)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `freeze_operator_api_requests_total` | Counter | `decision`, `namespace`, `kind`, `action` | API evaluate requests |
| `freeze_operator_api_latency_seconds` | Histogram | — | Evaluate request duration |
| `freeze_operator_api_errors_total` | Counter | `error_type` | API errors |

### Example PromQL

```promql
# Denied requests per hour
rate(freeze_operator_denied_requests_total[1h])

# Active freezes
freeze_operator_active_policies_total{policy_type="changefreeze"}

# API request rate
rate(freeze_operator_api_requests_total[5m])

# API P95 latency
histogram_quantile(0.95, freeze_operator_api_latency_seconds_bucket)
```

---

## Webhook Configuration

**Endpoint:** `workloads.freeze-operator.io`

**Watched Resources:**

- `apps/v1` — Deployment, StatefulSet, DaemonSet (CREATE, UPDATE, DELETE)
- `batch/v1` — CronJob (CREATE, UPDATE, DELETE)
- `*/scale` subresource (UPDATE)

**Failure Policy:** `Fail` (configurable)

**Namespace Exclusions:** `kube-system`, `kube-public`, `kube-node-lease`

---

## Upgrade Path

- [v1.0 → v2.0](upgrade-v1.0-to-v2.0.md) — GitOps integration
- [v2.0 → v3.0](upgrade-v2.0-to-v3.0.md) — CI Helper API
- [v3.0 → v3.0.1](upgrade-v3.0-to-v3.0.1.md) — API authentication

---

## See Also

- [Usage Guide](usage.md) - Practical examples
- [Architecture](architecture.md) - System design
- [CI Helper API](ci-api.md) - REST API reference and authentication
- [Troubleshooting](troubleshooting.md) - Common issues
