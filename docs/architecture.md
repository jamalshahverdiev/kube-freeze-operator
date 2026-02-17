# Architecture

## Overview

kube-freeze-operator consists of three main components:

```txt
┌─────────────────────────────────────────────────────┐
│                  Kubernetes API                     │
└─────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────┐
│           kube-freeze-operator Manager              │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │ Controllers  │  │   Webhooks   │  │  Metrics │ │
│  └──────────────┘  └──────────────┘  └──────────┘ │
└─────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────┐
│              Workload Resources                     │
│  (Deployments, StatefulSets, DaemonSets, CronJobs) │
└─────────────────────────────────────────────────────┘
```

## Components

### 1. Custom Resource Definitions (CRDs)

#### MaintenanceWindow

- **Scope**: Cluster-wide
- **Purpose**: Define recurring time windows when changes are allowed
- **Key Features**:
  - Cron-based scheduling
  - Timezone support
  - Multiple windows per policy
  - Mode: DenyOutsideWindows

#### ChangeFreeze

- **Scope**: Cluster-wide
- **Purpose**: Block changes during specific time periods
- **Key Features**:
  - Fixed start/end times (RFC3339)
  - Optional timezone
  - Time remaining calculation

#### FreezeException

- **Scope**: Cluster-wide
- **Purpose**: Override freeze policies for emergency changes
- **Key Features**:
  - Active time period
  - Constrained by labels/users/groups
  - Audit trail (reason, approver, ticket)

### 2. Controllers

#### MaintenanceWindowReconciler

- Evaluates cron schedules against current time
- Updates status with active/next windows
- Optionally suspends/resumes CronJobs
- Requeues at state transition times

#### ChangeFreezeReconciler

- Checks if current time is within freeze period
- Calculates time remaining
- Updates status and conditions
- Manages CronJob suspension during freeze

#### FreezeExceptionReconciler

- Tracks exception active state
- Updates status and conditions
- Minimal reconciliation logic

### 3. Admission Webhooks

#### CRD Validation Webhooks

Located in `internal/webhook/v1alpha1/`

- Validate timezone strings
- Parse and validate cron schedules
- Enforce time ordering (endTime > startTime)
- Check duration values

#### Workload Admission Webhook

Located in `internal/webhook/workloads/`:

- **Path**: `/validate-freeze-operator-io-v1alpha1-workloads`
- **Resources**: Deployments, StatefulSets, DaemonSets, CronJobs
- **Operations**: CREATE, UPDATE, DELETE
- **Special**: Handles `/scale` subresource

**Flow**:

1. Extract operation details (kind, action, labels)
2. Fetch namespace labels
3. Call policy evaluator
4. Allow or deny with formatted message

### 4. Policy Evaluator

Located in `internal/policy/evaluator.go`

**Decision Process**:

```txt
1. Collect active deny policies
   ├─ ChangeFreeze (if time in [start, end])
   └─ MaintenanceWindow (if DenyOutsideWindows and not in window)

2. If deny policies found:
   └─ Check for FreezeException override
      ├─ Match selectors (namespace, object, kind)
      ├─ Check action is in allow list
      ├─ Verify constraints (labels, users, groups)
      └─ If match: ALLOW, else: DENY

3. If no deny policies: ALLOW
```

**Priority**: When multiple policies match, selects one with earliest `nextAllowedTime`

### 5. Diff Detection

Located in `internal/diff/classify.go`

Classifies UPDATE operations:

- **ROLL_OUT**: `spec.template` changed
- **SCALE**: `spec.replicas` changed
- **CronJob**: Any `spec` change → ROLL_OUT

### 6. CronJob Management

Located in `internal/controller/cronjob_helper.go`

**Annotations**:

- `freeze-operator.io/managed-by`: Policy name
- `freeze-operator.io/original-suspend`: Original suspend state

**Logic**:

1. Find matching CronJobs (by selectors)
2. If first time managing:
   - Save original suspend state
   - Set managed-by annotation
3. Update suspend field based on policy state
4. On deactivation: Restore original state

**Conflict Prevention**: Skips CronJobs managed by different policy

## Data Flow

### Admission Request Flow

```txt
1. kubectl apply deployment.yaml
              ↓
2. API Server → Admission Webhook
              ↓
3. Extract request details
   - Kind, Namespace, Labels
   - Operation (CREATE/UPDATE/DELETE)
   - Old/New objects (for UPDATE)
              ↓
4. Classify action
   - CREATE → ActionCreate
   - DELETE → ActionDelete
   - UPDATE → Diff detection (ROLL_OUT/SCALE)
              ↓
5. Policy Evaluator
   - List MaintenanceWindows, ChangeFreezes
   - Match by selectors (namespace, object, kinds)
   - Evaluate time conditions
   - Check FreezeExceptions
              ↓
6. Decision: ALLOW or DENY
   - If DENY: Format message with policy info
   - If ALLOW: Log metrics
              ↓
7. Return Admission Response
```

### Controller Reconciliation Flow

```txt
1. Watch CRD changes
        ↓
2. Reconcile triggered
   - Fetch resource
   - Evaluate current state
        ↓
3. Update Status
   - Set active field
   - Update conditions
   - Calculate next window/time
        ↓
4. Side Effects (if enabled)
   - Suspend/resume CronJobs
   - Create events
   - Update metrics
        ↓
5. Requeue at next state change
```

## Security Considerations

### Webhook Security

- **TLS**: Required (managed by cert-manager)
- **failurePolicy**: Configurable (Fail recommended for production)
- **Bypass**: Operator serviceaccount bypasses enforcement
- **Terminating Namespaces**: Always allowed (prevents deadlocks)

### RBAC

- **Controllers**: Minimal permissions (get/list/watch + CronJob patch)
- **Webhooks**: Read-only for policy evaluation
- **Users**: Separate roles for viewing/editing policies

### Audit

- **Events**: Policy activation/deactivation
- **Metrics**: Denied/allowed requests, exception usage
- **Logs**: Structured logging with policy context

## Performance

### Scalability

- **Policies**: O(n) evaluation where n = number of policies
- **Namespaces**: Efficient label selector matching
- **CronJobs**: Batched updates per namespace

### Caching

- Uses controller-runtime cache for API reads
- Webhook uses APIReader for live reads (bypasses cache)

### Requeue Strategy

- Dynamic requeue times based on next state change
- Minimum: 1 second after state change
- Default fallback: 5 minutes

## Monitoring

### Metrics (Prometheus)

- `freeze_operator_active_policies_total`
- `freeze_operator_denied_requests_total`
- `freeze_operator_allowed_requests_total`
- `freeze_operator_exception_overrides_total`
- `freeze_operator_reconciliation_duration_seconds`
- `freeze_operator_cronjob_suspensions_total`

### Health Checks

- `/healthz`: Liveness probe
- `/readyz`: Readiness probe
- `/metrics`: Prometheus metrics endpoint

## Extension Points

### Future Enhancements

1. **GitOps Integration** (v2.0): Pause ArgoCD/Flux applications
2. **More Resources**: Jobs, Pods (via controllers)
3. **Advanced Scheduling**: Exclude holidays, custom calendars
4. **Notification**: Slack/email alerts
5. **UI Dashboard**: Visual policy management
