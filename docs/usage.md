# Usage Guide

This guide covers common use cases and examples for kube-freeze-operator.

## Table of Contents

- [Basic Concepts](#basic-concepts)
- [MaintenanceWindow Examples](#maintenancewindow-examples)
- [ChangeFreeze Examples](#changefreeze-examples)
- [FreezeException Examples](#freezeexception-examples)
- [Best Practices](#best-practices)
- [Common Patterns](#common-patterns)

## Basic Concepts

### Policy Types

| Policy | When to Use | Blocks Changes |
|--------|-------------|----------------|
| **MaintenanceWindow** | Define regular maintenance windows (e.g., nightly) | OUTSIDE windows |
| **ChangeFreeze** | Block changes during specific periods (e.g., holidays) | INSIDE period |
| **FreezeException** | Allow emergency changes despite freeze | Never (overrides) |

### Actions

- `CREATE`: Creating new resources
- `DELETE`: Deleting resources
- `ROLL_OUT`: Changing PodTemplate (image, env, etc.)
- `SCALE`: Changing replica count

### Target Selectors

```yaml
target:
  # Select namespaces by labels
  namespaceSelector:
    matchLabels:
      env: prod
  
  # Select objects by labels (for exceptions)
  objectSelector:
    matchLabels:
      emergency: "true"
  
  # Resource types to apply policy to
  kinds:
    - Deployment
    - StatefulSet
    - DaemonSet
    - CronJob
```

## MaintenanceWindow Examples

### Example 1: Nightly Maintenance Window

Allow changes only during night hours (1 AM - 7 AM UTC):

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: nightly-maintenance
spec:
  timezone: UTC
  mode: DenyOutsideWindows
  windows:
    - name: night-window
      schedule: "0 1 * * *"  # Daily at 1 AM
      duration: 6h            # 6 hour window
  target:
    namespaceSelector:
      matchLabels:
        env: prod
    kinds: [Deployment, StatefulSet, DaemonSet, CronJob]
  rules:
    deny: [ROLL_OUT, SCALE, CREATE, DELETE]
  behavior:
    suspendCronJobs: true
  message:
    reason: "Production changes allowed only during nightly maintenance window"
    docsURL: "https://wiki.company.com/maintenance-windows"
    contact: "#platform-team"
```

### Example 2: Business Hours Window

Allow changes only during business hours (Mon-Fri, 9 AM - 5 PM):

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: business-hours
spec:
  timezone: America/New_York
  mode: DenyOutsideWindows
  windows:
    - name: weekday-morning
      schedule: "0 9 * * 1-5"  # Mon-Fri at 9 AM
      duration: 8h              # Until 5 PM
  target:
    namespaceSelector:
      matchLabels:
        env: prod
        team: platform
    kinds: [Deployment, StatefulSet]
  rules:
    deny: [ROLL_OUT, CREATE, DELETE]
  message:
    reason: "Changes allowed only during business hours (Mon-Fri 9AM-5PM EST)"
```

### Example 3: Multiple Windows Per Day

Allow changes during multiple time slots:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: multi-window
spec:
  timezone: Europe/London
  mode: DenyOutsideWindows
  windows:
    - name: morning-window
      schedule: "0 8 * * *"
      duration: 2h
    - name: afternoon-window
      schedule: "0 14 * * *"
      duration: 2h
    - name: evening-window
      schedule: "0 20 * * *"
      duration: 2h
  target:
    namespaceSelector:
      matchLabels:
        env: staging
    kinds: [Deployment, StatefulSet, DaemonSet, CronJob]
  rules:
    deny: [ROLL_OUT, SCALE]
```

## ChangeFreeze Examples

### Example 1: Holiday Freeze

Block all changes during holiday period:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: christmas-freeze
spec:
  startTime: "2026-12-24T00:00:00Z"
  endTime: "2026-12-27T00:00:00Z"
  target:
    namespaceSelector:
      matchLabels:
        env: prod
    kinds: [Deployment, StatefulSet, DaemonSet, CronJob]
  rules:
    deny: [ROLL_OUT, SCALE, CREATE, DELETE]
  behavior:
    suspendCronJobs: true
  message:
    reason: "Holiday change freeze - emergency changes require exception"
    docsURL: "https://wiki.company.com/freeze-policy"
    contact: "#sre-oncall"
```

### Example 2: End-of-Quarter Freeze

Freeze during critical business period:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: q1-end-freeze
spec:
  startTime: "2026-03-25T00:00:00Z"
  endTime: "2026-04-01T00:00:00Z"
  timezone: "America/New_York"
  target:
    namespaceSelector:
      matchLabels:
        env: prod
        criticality: high
    kinds: [Deployment, StatefulSet]
  rules:
    deny: [ROLL_OUT, DELETE]  # Allow SCALE for load handling
  message:
    reason: "End of quarter freeze - critical period for financial reporting"
    contact: "#business-operations"
```

### Example 3: Deployment Freeze (Allow Scaling)

Block deployments but allow scaling for traffic:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: deployment-freeze-allow-scale
spec:
  startTime: "2026-02-20T14:00:00Z"
  endTime: "2026-02-20T18:00:00Z"
  target:
    namespaceSelector:
      matchLabels:
        env: prod
    kinds: [Deployment, StatefulSet]
  rules:
    deny: [ROLL_OUT, CREATE, DELETE]
    # SCALE is NOT in deny list, so it's allowed
  message:
    reason: "Marketing campaign - deployments frozen but scaling allowed"
```

## FreezeException Examples

### Example 1: Emergency Hotfix

Allow hotfix deployment during freeze:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: FreezeException
metadata:
  name: critical-security-fix
spec:
  activeFrom: "2026-02-17T10:00:00Z"
  activeTo: "2026-02-17T12:00:00Z"
  target:
    namespaceSelector:
      matchLabels:
        env: prod
    objectSelector:
      matchLabels:
        app: payment-service
        hotfix: "CVE-2026-1234"
    kinds: [Deployment]
  allow: [ROLL_OUT]
  reason: "Emergency security patch for CVE-2026-1234"
  ticketURL: "https://jira.company.com/INC-12345"
  approvedBy: "john.doe@company.com"
  constraints:
    allowedUsers:
      - "john.doe@company.com"
      - "jane.smith@company.com"
    allowedGroups:
      - "sre-team"
```

### Example 2: Planned Exception for Specific Team

Pre-approved exception for database team:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: FreezeException
metadata:
  name: db-team-maintenance
spec:
  activeFrom: "2026-02-20T00:00:00Z"
  activeTo: "2026-02-21T00:00:00Z"
  target:
    namespaceSelector:
      matchLabels:
        team: database
    objectSelector:
      matchLabels:
        component: postgres
    kinds: [StatefulSet]
  allow: [ROLL_OUT, SCALE]
  reason: "Planned database upgrade during freeze period"
  ticketURL: "https://jira.company.com/MAINT-456"
  approvedBy: "db-lead@company.com"
  constraints:
    requireLabels:
      approved-change: "true"
    allowedGroups:
      - "dba-team"
```

### Example 3: Label-Based Emergency Exception

Exception for workloads marked as emergency:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: FreezeException
metadata:
  name: emergency-label-exception
spec:
  activeFrom: "2026-02-17T00:00:00Z"
  activeTo: "2026-02-18T00:00:00Z"
  target:
    objectSelector:
      matchLabels:
        emergency: "true"
        approved: "true"
    kinds: [Deployment, StatefulSet]
  allow: [ROLL_OUT, SCALE, CREATE]
  reason: "Emergency changes for workloads with emergency=true label"
  constraints:
    requireLabels:
      emergency: "true"
      approved: "true"
    allowedGroups:
      - "sre-oncall"
      - "platform-admin"
```

## Best Practices

### 1. Start Permissive, Then Restrict

Begin with informational policies (logs/metrics only) before enforcing:

```yaml
# Phase 1: Monitor (deploy but don't block)
# Use metrics to understand change patterns

# Phase 2: Enforce on staging
namespaceSelector:
  matchLabels:
    env: staging

# Phase 3: Enforce on production
namespaceSelector:
  matchLabels:
    env: prod
```

### 2. Layer Policies

Combine MaintenanceWindow and ChangeFreeze:

```yaml
# Base policy: Allow changes only during maintenance windows
MaintenanceWindow: "nightly-maintenance"

# Override: Holiday freeze blocks even maintenance windows
ChangeFreeze: "holiday-freeze"

# Exception: Emergency changes can override both
FreezeException: "critical-hotfix"
```

### 3. Use Meaningful Labels

Label your namespaces and workloads:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: production
  labels:
    env: prod
    team: platform
    criticality: high
    freeze-policy: strict
```

### 4. Document Exceptions

Always provide context in exceptions:

```yaml
spec:
  reason: "Critical security patch for CVE-2026-XXXX"
  ticketURL: "https://jira.company.com/INC-12345"
  approvedBy: "security-lead@company.com"
```

### 5. Monitor Metrics

Track denied requests and exception usage:

```promql
# Denied requests by policy
rate(freeze_operator_denied_requests_total[5m])

# Exception usage frequency
freeze_operator_exception_overrides_total

# Active freeze policies
freeze_operator_active_policies_total
```

### 6. Set Up Alerts

Alert on unexpected patterns:

```yaml
- alert: HighFreezeDenialRate
  expr: rate(freeze_operator_denied_requests_total[5m]) > 10
  for: 5m
  annotations:
    summary: "High rate of denied requests due to freeze"

- alert: ExcessiveExceptionUsage
  expr: increase(freeze_operator_exception_overrides_total[1h]) > 5
  annotations:
    summary: "Unusual number of freeze exceptions"
```

## Common Patterns

### Pattern 1: Progressive Rollout During Freeze

Allow scaling but block deployments:

```yaml
rules:
  deny: [ROLL_OUT, CREATE, DELETE]
  # SCALE not in list = allowed
```

### Pattern 2: Team-Specific Policies

Different policies for different teams:

```yaml
# Team A: Strict freeze
target:
  namespaceSelector:
    matchLabels:
      team: frontend
  rules:
    deny: [ROLL_OUT, SCALE, CREATE, DELETE]

# Team B: Allow scaling
target:
  namespaceSelector:
    matchLabels:
      team: backend
  rules:
    deny: [ROLL_OUT, CREATE, DELETE]
```

### Pattern 3: Graduated Freeze Levels

Multiple environments with different policies:

```yaml
# Production: Strict
env: prod → deny: [ROLL_OUT, SCALE, CREATE, DELETE]

# Staging: Moderate  
env: staging → deny: [ROLL_OUT, DELETE]

# Development: Permissive
env: dev → No freeze policy
```

## Troubleshooting Scenarios

### "Why was my deployment denied?"

1. Check active policies:

```bash
kubectl get maintenancewindows,changefreezes -A
```

1. Check policy status:

```bash
kubectl get maintenancewindow nightly-maintenance -o yaml
# Look at status.active and status.nextWindow
```

1. View webhook logs:

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager -c manager
```

### "How do I allow emergency changes?"

Create a FreezeException:

```bash
kubectl apply -f - <<EOF
apiVersion: freeze-operator.io/v1alpha1
kind: FreezeException
metadata:
  name: hotfix-$(date +%s)
spec:
  activeFrom: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  activeTo: "$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)"
  target:
    objectSelector:
      matchLabels:
        app: my-app
    kinds: [Deployment]
  allow: [ROLL_OUT]
  reason: "Emergency hotfix"
  approvedBy: "$(whoami)"
EOF
```

### "How do I temporarily disable enforcement?"

**Option 1**: Delete the policy:

```bash
kubectl delete maintenancewindow <name>
```

**Option 2**: Set webhook failurePolicy to Ignore (NOT recommended for production):

```bash
kubectl patch validatingwebhookconfigurations \
  kube-freeze-operator-validating-webhook-configuration-workloads \
  --type merge -p '{"webhooks":[{"name":"workloads.freeze-operator.io","failurePolicy":"Ignore"}]}'
```

## Next Steps

- Review [Troubleshooting Guide](troubleshooting.md)
- Check [API Reference](api-reference.md)
- See [Examples](../examples/)
