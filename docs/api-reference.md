# API Reference

Complete API reference for kube-freeze-operator CRDs.

## API Versions

- **v1alpha1**: Current stable API version

## Core Types

### MaintenanceWindow

**API Group:** freeze-operator.io  
**API Version:** v1alpha1  
**Kind:** MaintenanceWindow  
**Scope:** Namespaced

Defines recurring time windows when specific actions are allowed based on cron schedules.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schedule` | string | Yes | Cron expression (5 fields): minute, hour, day-of-month, month, day-of-week. Example: `"0 1 * * *"` (daily at 1 AM) |
| `duration` | string | Yes | How long the window stays open. Format: `<number><unit>` where unit is `s`, `m`, `h`. Examples: `"30m"`, `"2h"`, `"90m"` |
| `timezone` | string | Yes | IANA timezone name. Examples: `"UTC"`, `"America/New_York"`, `"Europe/London"` |
| `policyRules` | [PolicyRulesSpec](#policyrulesspec) | Yes | Defines what actions are allowed during this window |
| `target` | [TargetSpec](#targetspec) | Yes | Specifies which namespaces/resources this window applies to |
| `message` | [MessageSpec](#messagespec) | No | Custom message for users when window is inactive |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Whether the window is currently open |
| `nextWindowStart` | string (time) | RFC3339 timestamp of next window opening |
| `nextWindowEnd` | string (time) | RFC3339 timestamp of next window closing |
| `lastScheduleTime` | string (time) | Last time the schedule was evaluated |
| `conditions` | []metav1.Condition | Standard Kubernetes conditions |

#### Conditions

| Type | Status | Reason | Description |
|------|--------|--------|-------------|
| `Ready` | True | ScheduleValid | Cron schedule is valid and working |
| `Ready` | False | InvalidSchedule | Cron schedule is malformed |
| `Ready` | False | InvalidTimezone | Timezone is not recognized |

#### Examples

See [usage.md - MaintenanceWindow examples](usage.md#maintenancewindow)

---

### ChangeFreeze

**API Group:** freeze-operator.io  
**API Version:** v1alpha1  
**Kind:** ChangeFreeze  
**Scope:** Namespaced

Defines fixed time periods when changes are blocked.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `start` | string (time) | Yes | RFC3339 timestamp when freeze begins. Example: `"2026-12-20T00:00:00Z"` |
| `end` | string (time) | Yes | RFC3339 timestamp when freeze ends. Must be after `start` |
| `policyRules` | [PolicyRulesSpec](#policyrulesspec) | Yes | Defines what actions are blocked during freeze |
| `target` | [TargetSpec](#targetspec) | Yes | Specifies which namespaces/resources this freeze applies to |
| `behavior` | [FreezeBehaviorSpec](#freezebehaviorspec) | No | Additional behavior like suspending CronJobs |
| `message` | [MessageSpec](#messagespec) | No | Custom message for users when freeze is active |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Whether the freeze is currently active |
| `phase` | string | Current phase: `Pending`, `Active`, or `Completed` |
| `conditions` | []metav1.Condition | Standard Kubernetes conditions |

#### Conditions

| Type | Status | Reason | Description |
|------|--------|--------|-------------|
| `Active` | True | WithinFreezePeriod | Currently within start/end time |
| `Active` | False | BeforeStart | Current time < start |
| `Active` | False | AfterEnd | Current time > end |

#### Examples

See [usage.md - ChangeFreeze examples](usage.md#changefreeze)

---

### FreezeException

**API Group:** freeze-operator.io  
**API Version:** v1alpha1  
**Kind:** FreezeException  
**Scope:** Namespaced

Override for allowing specific changes during a freeze.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `changeFreezeName` | string | Yes | Name of the ChangeFreeze to bypass. Must exist in same namespace |
| `allow` | [PolicyRulesSpec](#policyrulesspec) | Yes | Defines what actions to allow despite the freeze |
| `target` | [TargetSpec](#targetspec) | Yes | Specifies which resources this exception applies to |
| `start` | string (time) | No | When exception becomes active. Defaults to immediate |
| `end` | string (time) | No | When exception expires. If omitted, lasts until ChangeFreeze ends |
| `reason` | string | Yes | Human-readable explanation for this exception. Min 10 chars |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `active` | boolean | Whether the exception is currently in effect |
| `conditions` | []metav1.Condition | Standard Kubernetes conditions |

#### Conditions

| Type | Status | Reason | Description |
|------|--------|--------|-------------|
| `Valid` | True | ChangeFreezeFound | Referenced ChangeFreeze exists |
| `Valid` | False | ChangeFreezeNotFound | Referenced ChangeFreeze doesn't exist |
| `Active` | True | WithinValidityPeriod | Currently within start/end time |
| `Active` | False | BeforeStart | Current time < start |
| `Active` | False | AfterEnd | Current time > end |

#### Examples

See [usage.md - FreezeException examples](usage.md#freezeexception)

---

## Common Types

### TargetSpec

Defines which namespaces and resources a policy applies to.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespaceSelector` | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta) | Yes | Selects namespaces by labels |
| `objectSelector` | [metav1.LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta) | No | Selects objects within matched namespaces by labels. If empty, applies to all objects |

**Examples:**

```yaml
# Match all namespaces with env=prod label
target:
  namespaceSelector:
    matchLabels:
      env: prod
```

```yaml
# Match namespaces with env=prod AND tier=frontend
target:
  namespaceSelector:
    matchLabels:
      env: prod
      tier: frontend
```

```yaml
# Match namespaces AND specific objects
target:
  namespaceSelector:
    matchLabels:
      env: prod
  objectSelector:
    matchLabels:
      app: critical-service
```

```yaml
# Match using matchExpressions
target:
  namespaceSelector:
    matchExpressions:
      - key: env
        operator: In
        values: ["prod", "staging"]
      - key: team
        operator: NotIn
        values: ["platform"]
```

**Supported Label Selector Operators:**

- `In`: Label value must be in list
- `NotIn`: Label value must not be in list
- `Exists`: Label key must exist (value ignored)
- `DoesNotExist`: Label key must not exist

---

### PolicyRulesSpec

Defines what actions are controlled by the policy.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `actions` | []string | Yes | List of actions to control. Valid values: `UPDATE`, `ROLL_OUT`, `SCALE` |
| `resources` | []string | Yes | List of resource types. Valid values: `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob` |

**Action Details:**

| Action | Description | Applies To |
|--------|-------------|------------|
| `UPDATE` | Any modification to resource spec | All resources |
| `ROLL_OUT` | Changes that trigger pod restart (image, env, etc.) | Deployment, StatefulSet, DaemonSet |
| `SCALE` | Replica count changes | Deployment, StatefulSet |

**Important:** `UPDATE` is broad and includes `ROLL_OUT` and `SCALE`. Use specific actions when possible.

**Examples:**

```yaml
# Block all updates to Deployments and StatefulSets
policyRules:
  actions:
    - UPDATE
  resources:
    - Deployment
    - StatefulSet
```

```yaml
# Block only rollouts (image/config changes), allow scaling
policyRules:
  actions:
    - ROLL_OUT
  resources:
    - Deployment
```

```yaml
# Block scaling, allow everything else
policyRules:
  actions:
    - SCALE
  resources:
    - Deployment
    - StatefulSet
```

```yaml
# Block all changes to all workload types
policyRules:
  actions:
    - UPDATE
  resources:
    - Deployment
    - StatefulSet
    - DaemonSet
    - CronJob
```

---

### FreezeBehaviorSpec

Additional behaviors during a freeze.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `suspendCronJobs` | boolean | No | false | If true, operator suspends all matching CronJobs during freeze and restores them after |

**How CronJob Suspension Works:**

1. When freeze activates:
   - Operator finds all CronJobs matching `target` selector
   - Stores original `suspend` value in annotation `freeze-operator.io/original-suspend`
   - Sets `spec.suspend: true`
   - Adds annotation `freeze-operator.io/managed-by: <freeze-name>`

2. When freeze deactivates:
   - Operator restores original `suspend` value
   - Removes management annotations

**Conflict Prevention:**
Only the first ChangeFreeze to manage a CronJob "owns" it. Other overlapping freezes won't modify that CronJob.

**Example:**

```yaml
spec:
  behavior:
    suspendCronJobs: true
  target:
    namespaceSelector:
      matchLabels:
        env: prod
  # ... rest of spec
```

---

### MessageSpec

Custom message shown to users when policy denies a request.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `whenInactive` | string | No | Message shown when MaintenanceWindow is closed. Default: "Outside maintenance window" |
| `whenActive` | string | No | Message shown when ChangeFreeze is active. Default: "Change freeze in effect" |

**Example:**

```yaml
message:
  whenActive: |
    Production is frozen for holiday season.
    Emergency changes require VP approval.
    Contact #ops-emergency on Slack.
```

**Message Formatting:**

- Supports multi-line strings
- Maximum recommended length: 200 characters
- Displayed in kubectl error output

---

## Validation Rules

### MaintenanceWindow

| Field | Validation |
|-------|------------|
| `schedule` | Must be valid 5-field cron expression |
| `duration` | Must parse as valid Go duration (e.g., `30m`, `2h`) |
| `duration` | Must be > 0 and < 24 hours |
| `timezone` | Must be valid IANA timezone |
| `policyRules.actions` | Must contain at least one action |
| `policyRules.resources` | Must contain at least one resource |
| `target.namespaceSelector` | Must not be empty |

**Invalid Examples:**

```yaml
# ❌ Invalid cron (6 fields)
schedule: "* * * * * *"

# ❌ Invalid duration
duration: "90"  # Missing unit

# ❌ Invalid timezone
timezone: "PST"  # Use "America/Los_Angeles"

# ❌ Empty actions
policyRules:
  actions: []
```

### ChangeFreeze

| Field | Validation |
|-------|------------|
| `start` | Must be valid RFC3339 timestamp |
| `end` | Must be valid RFC3339 timestamp |
| `end` | Must be after `start` |
| `policyRules.actions` | Must contain at least one action |
| `policyRules.resources` | Must contain at least one resource |
| `target.namespaceSelector` | Must not be empty |

**Invalid Examples:**

```yaml
# ❌ End before start
start: "2026-12-20T00:00:00Z"
end: "2026-12-19T00:00:00Z"

# ❌ Invalid timestamp format
start: "2026-12-20"  # Missing time
```

### FreezeException

| Field | Validation |
|-------|------------|
| `changeFreezeName` | Must not be empty |
| `reason` | Must be at least 10 characters |
| `end` | If specified, must be after `start` |
| `allow.actions` | Must contain at least one action |
| `allow.resources` | Must contain at least one resource |
| `target.namespaceSelector` | Must not be empty |

**Invalid Examples:**

```yaml
# ❌ Reason too short
reason: "hotfix"

# ❌ Empty changeFreezeName
changeFreezeName: ""
```

---

## Priority and Conflict Resolution

When multiple policies match a request, the operator uses this priority order:

1. **FreezeException** (highest priority) - allows changes
2. **ChangeFreeze** - denies changes
3. **MaintenanceWindow** - allows changes
4. **No match** - default allow

**Examples:**

**Scenario 1: Exception overrides freeze**

- ChangeFreeze blocks all updates
- FreezeException allows updates to app=backend
- Result: app=backend deployments can be updated, others blocked

**Scenario 2: Window vs freeze (no exception)**

- MaintenanceWindow allows updates 1-3 AM
- ChangeFreeze blocks updates all day
- Result: Freeze wins, updates blocked even during window

**Scenario 3: Multiple freezes**

- ChangeFreeze A blocks ROLL_OUT
- ChangeFreeze B blocks SCALE
- Result: Both ROLL_OUT and SCALE blocked

**Scenario 4: Overlapping targets**

- Policy A: `env=prod` → blocks updates
- Policy B: `env=prod, tier=web` → blocks updates
- Result: Both policies evaluated, stricter rules apply

---

## Annotations

The operator adds annotations to managed resources:

| Annotation | Added To | Description |
|------------|----------|-------------|
| `freeze-operator.io/managed-by` | CronJob | Name of ChangeFreeze managing this CronJob |
| `freeze-operator.io/original-suspend` | CronJob | Original value of `spec.suspend` before operator modified it |

**Example:**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: backup
  annotations:
    freeze-operator.io/managed-by: "holiday-freeze"
    freeze-operator.io/original-suspend: "false"
spec:
  suspend: true  # Set by operator
```

**Do not modify these annotations manually** - they are managed by the operator.

---

## Events

The operator emits Kubernetes events:

| Type | Reason | Description |
|------|--------|-------------|
| Normal | MaintenanceWindowOpened | MaintenanceWindow became active |
| Normal | MaintenanceWindowClosed | MaintenanceWindow became inactive |
| Normal | FreezeFreezeActivated | ChangeFreeze entered active phase |
| Normal | ChangeFreezeFreezeDeactivated | ChangeFreeze completed |
| Normal | ExceptionActivated | FreezeException became active |
| Normal | CronJobSuspended | Operator suspended a CronJob |
| Normal | CronJobResumed | Operator restored CronJob suspend state |
| Warning | InvalidSchedule | Cron schedule is malformed |
| Warning | InvalidTimezone | Timezone is not recognized |

**View events:**

```bash
# For a specific resource
kubectl describe maintenancewindow <name>

# All operator events
kubectl get events --field-selector source=kube-freeze-operator-controller-manager
```

---

## Metrics

The operator exposes Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `freeze_operator_active_policies_total` | Gauge | Number of currently active policies by type |
| `freeze_operator_denied_requests_total` | Counter | Total admission requests denied |
| `freeze_operator_allowed_requests_total` | Counter | Total admission requests allowed |
| `freeze_operator_reconciliation_duration_seconds` | Histogram | Time spent in reconciliation loops |

**Labels:**

- `policy_type`: `MaintenanceWindow`, `ChangeFreeze`, `FreezeException`
- `namespace`: Target namespace
- `resource_type`: `Deployment`, `StatefulSet`, etc.
- `action`: `UPDATE`, `ROLL_OUT`, `SCALE`

**Example PromQL queries:**

```promql
# Denied requests per hour
rate(freeze_operator_denied_requests_total[1h])

# Active freezes
freeze_operator_active_policies_total{policy_type="ChangeFreeze"}

# P99 reconciliation latency
histogram_quantile(0.99, freeze_operator_reconciliation_duration_seconds_bucket)
```

---

## Webhook Configuration

The operator registers a validating admission webhook:

**Endpoint:** `workloads.freeze-operator.io`

**Watched Resources:**

- `apps/v1/Deployment` (CREATE, UPDATE)
- `apps/v1/StatefulSet` (CREATE, UPDATE)
- `apps/v1/DaemonSet` (CREATE, UPDATE)
- `batch/v1/CronJob` (CREATE, UPDATE)
- `*/scale` subresource (UPDATE)

**Failure Policy:** Configurable (default: `Fail`)

- `Fail`: Reject requests if webhook unavailable (recommended for production)
- `Ignore`: Allow requests if webhook unavailable (useful for development)

**Namespace Exclusions:**

- `kube-system`
- `kube-public`
- `kube-node-lease`

---

## RBAC

Required ClusterRole permissions:

```yaml
# For CRD management
apiGroups: ["freeze-operator.io"]
resources: ["maintenancewindows", "changefreezes", "freezeexceptions"]
verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# For status updates
apiGroups: ["freeze-operator.io"]
resources: ["maintenancewindows/status", "changefreezes/status", "freezeexceptions/status"]
verbs: ["get", "update", "patch"]

# For CronJob suspension
apiGroups: ["batch"]
resources: ["cronjobs"]
verbs: ["get", "list", "watch", "update", "patch"]

# For reading workload metadata
apiGroups: ["apps"]
resources: ["deployments", "statefulsets", "daemonsets"]
verbs: ["get", "list", "watch"]

# For events
apiGroups: [""]
resources: ["events"]
verbs: ["create", "patch"]
```

---

## Upgrade Path

### From Future Versions

Version 2.0 (planned):

- API group remains `freeze-operator.io`
- New API version: `v1beta1` or `v1`
- `v1alpha1` will be supported with conversion webhooks
- No manual migration required

---

## See Also

- [Usage Guide](usage.md) - Practical examples
- [Architecture](architecture.md) - System design
- [Troubleshooting](troubleshooting.md) - Common issues
