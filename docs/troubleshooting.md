# Troubleshooting Guide

Common issues and their solutions when working with kube-freeze-operator.

## Table of Contents

- [Webhook Issues](#webhook-issues)
- [Policy Not Working](#policy-not-working)
- [CronJob Issues](#cronjob-issues)
- [GitOps Integration Issues](#gitops-integration-issues)
- [Performance Issues](#performance-issues)
- [Debugging Tools](#debugging-tools)

## Webhook Issues

### Problem: Deployments hang with "context deadline exceeded"

**Symptoms:**

```txt
Error from server (InternalError): Internal error occurred: failed calling webhook
"workloads.freeze-operator.io": Post "https://kube-freeze-operator-webhook-service...": 
context deadline exceeded
```

**Causes:**

1. Webhook service not running
2. Certificate issues
3. Network policies blocking traffic

**Solutions:**

1. Check operator pods:

```bash
kubectl get pods -n kube-freeze-operator-system
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager
```

1. Check webhook service and endpoints:

```bash
kubectl get svc,endpoints -n kube-freeze-operator-system
```

1. Verify certificate:

```bash
kubectl get certificate -n kube-freeze-operator-system
kubectl get secret webhook-server-cert -n kube-freeze-operator-system
```

1. Check cert-manager:

```bash
kubectl get pods -n cert-manager
kubectl logs -n cert-manager deployment/cert-manager
```

1. Verify webhook configuration:

```bash
kubectl get validatingwebhookconfigurations | grep kube-freeze
kubectl get validatingwebhookconfigurations \
  kube-freeze-operator-validating-webhook-configuration-workloads -o yaml
```

### Problem: "x509: certificate signed by unknown authority"

**Cause:** CA bundle not injected into webhook configuration

**Solution:**

1. Verify cert-manager annotations:

```bash
kubectl get validatingwebhookconfigurations \
  kube-freeze-operator-validating-webhook-configuration-workloads \
  -o jsonpath='{.metadata.annotations}'
```

Should have: `cert-manager.io/inject-ca-from: kube-freeze-operator-system/serving-cert`

1. Check certificate status:

```bash
kubectl describe certificate -n kube-freeze-operator-system serving-cert
```

1. Re-deploy if needed:

```bash
make deploy IMG=<your-image>
```

### Problem: Webhook bypassed, changes go through despite freeze

**Causes:**

1. `failurePolicy: Ignore` set
2. Webhook service unavailable
3. User has bypass permissions

**Solutions:**

1. Check failurePolicy:

```bash
kubectl get validatingwebhookconfigurations \
  kube-freeze-operator-validating-webhook-configuration-workloads \
  -o jsonpath='{.webhooks[0].failurePolicy}'
```

Should be `Fail` for production.

1. Verify webhook is called:

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep "webhook"
```

## Policy Not Working

### Problem: Policy created but not enforcing

**Diagnosis Steps:**

1. Check policy status:

```bash
kubectl get maintenancewindow <name> -o yaml
```

Look at `status.active` and `status.conditions`.

1. Verify timezone:

```bash
# Test timezone validity
TZ=Europe/London date
```

1. Check cron schedule:

```bash
# Use online cron validator
# Example: https://crontab.guru/#0_1_*_*_*
```

1. Review target selectors:

```bash
# Check namespace labels
kubectl get ns <namespace> --show-labels

# Check object labels  
kubectl get deployment <name> -n <namespace> --show-labels
```

1. Verify the policy matches your namespace:

```bash
# Example: If policy has namespaceSelector.matchLabels.env=prod
kubectl get ns <your-namespace> -o jsonpath='{.metadata.labels.env}'
```

**Common Mistakes:**

- **Wrong timezone**: `America/NewYork` should be `America/New_York`
- **Invalid cron**: `* * * * * *` (6 fields) should be `* * * * *` (5 fields)
- **Selector mismatch**: Policy targets `env=prod`, namespace has `environment=production`

### Problem: MaintenanceWindow shows wrong active status

**Cause:** Timezone or schedule misconfiguration

**Debug:**

```bash
# Get current status
kubectl get maintenancewindow <name> -o jsonpath='{.status}'

# Check controller logs
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep -i "maintenancewindow.*<name>"
```

**Solution:**

1. Verify current time in operator's timezone:

```bash
kubectl exec -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  -- date
```

1. Test cron expression locally:

```go
// Use robfig/cron library
schedule := "0 1 * * *"
parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
sch, _ := parser.Parse(schedule)
next := sch.Next(time.Now())
fmt.Println(next)
```

### Problem: ChangeFreeze shows active but webhook allows changes

**Diagnosis:**

1. Check if exception exists:

```bash
kubectl get freezeexceptions
kubectl get freezeexception <name> -o yaml
```

1. Verify namespace/object labels match:

```bash
kubectl get deployment <name> -n <namespace> -o jsonpath='{.metadata.labels}'
```

1. Check webhook logs for decision:

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep "denied\|allowed"
```

## CronJob Issues

### Problem: CronJobs not suspending during freeze

**Diagnosis:**

1. Check if behavior is enabled:

```bash
kubectl get changefreeze <name> -o jsonpath='{.spec.behavior.suspendCronJobs}'
```

Should be `true`.

1. Verify CronJob matches target selector:

```bash
kubectl get cronjob <name> -n <namespace> --show-labels
```

1. Check CronJob annotations:

```bash
kubectl get cronjob <name> -n <namespace> \
  -o jsonpath='{.metadata.annotations}'
```

Should have:

- `freeze-operator.io/managed-by`
- `freeze-operator.io/original-suspend`

**Solution:**

If not working, check controller logs:

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep -i "cronjob"
```

### Problem: CronJob stays suspended after freeze ends

**Cause:** Operator couldn't restore original state

**Solution:**

1. Check CronJob annotations:

```bash
kubectl get cronjob <name> -n <namespace> \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/original-suspend}'
```

1. Manually restore if needed:

```bash
kubectl patch cronjob <name> -n <namespace> \
  --type merge -p '{"spec":{"suspend":false}}'

# Remove management annotations
kubectl annotate cronjob <name> -n <namespace> \
  freeze-operator.io/managed-by- \
  freeze-operator.io/original-suspend-
```

### Problem: Multiple policies conflict on same CronJob

**Symptom:** CronJob suspend state flapping

**Solution:**

The operator implements conflict prevention - only the first policy to manage a CronJob "owns" it. Check which policy manages it:

```bash
kubectl get cronjob <name> -n <namespace> \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed-by}'
```

To transfer ownership:

1. Delete the managing policy
2. CronJob will be restored
3. New policy will take over on next reconciliation

## GitOps Integration Issues

### Problem: ArgoCD shows "OutOfSync" during freeze

**Cause:** ArgoCD tries to sync changes that webhook blocks

**Solutions:**

**Option 1: Ignore OutOfSync during freeze** (v1.0 approach)

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  annotations:
    notifications.argoproj.io/subscribe.on-sync-failed.slack: "ignore-during-freeze"
```

**Option 2: Wait for v2.0**
Version 2.0 will automatically pause ArgoCD applications during freeze.

**Workaround:**
Temporarily disable auto-sync:

```bash
argocd app set <app-name> --sync-policy none
```

### Problem: Flux reconciliation fails during freeze

**Cause:** Similar to ArgoCD - Flux can't apply changes

**Workaround:**

```bash
flux suspend kustomization <name>
# After freeze ends:
flux resume kustomization <name>
```

## Performance Issues

### Problem: High CPU/Memory usage

**Diagnosis:**

```bash
kubectl top pods -n kube-freeze-operator-system
```

**Causes:**

1. Too many policies
2. Excessive reconciliation
3. Large number of CronJobs

**Solutions:**

1. Check reconciliation frequency:

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep "Reconcile"
```

1. Optimize policies:

- Use more specific selectors
- Reduce number of overlapping policies
- Increase requeue intervals if needed

1. Monitor metrics:

```bash
kubectl port-forward -n kube-freeze-operator-system \
  svc/kube-freeze-operator-controller-manager-metrics-service 8443:8443
  
curl -k https://localhost:8443/metrics | grep freeze_operator
```

### Problem: Slow admission webhook response

**Diagnosis:**

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  | grep "webhook.*duration"
```

**Solutions:**

1. Check if API server cache is working
2. Reduce number of active policies
3. Optimize label selectors

## Debugging Tools

### 1. Enable Debug Logging

Edit operator deployment:

```bash
kubectl edit deployment -n kube-freeze-operator-system \
  kube-freeze-operator-controller-manager
```

Add/change args:

```yaml
args:
  - --zap-log-level=debug
  - --zap-devel=true
```

### 2. Check Metrics

```bash
# Port-forward metrics service
kubectl port-forward -n kube-freeze-operator-system \
  svc/kube-freeze-operator-controller-manager-metrics-service 8443:8443

# Query metrics
curl -k https://localhost:8443/metrics | grep freeze_operator

# Key metrics to check:
# - freeze_operator_active_policies_total
# - freeze_operator_denied_requests_total
# - freeze_operator_reconciliation_duration_seconds
```

### 3. Inspect Events

```bash
# Operator events
kubectl get events -n kube-freeze-operator-system \
  --sort-by='.lastTimestamp'

# Policy events
kubectl get events --all-namespaces \
  --field-selector involvedObject.kind=MaintenanceWindow

# Webhook events
kubectl get events --all-namespaces | grep "freeze-operator"
```

### 4. Test Policy Manually

```bash
# Create test deployment
kubectl create deployment test --image=nginx -n test-namespace

# Try to scale it
kubectl scale deployment test --replicas=3 -n test-namespace

# Should be denied if freeze active
```

### 5. View Policy Evaluation

```bash
# Watch controller logs in real-time
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  -f \
  | grep -E "denied|allowed|policy|webhook"
```

### 6. Validate CRDs

```bash
# Check if CRDs are installed
kubectl get crds | grep freeze-operator.io

# Validate CRD schema
kubectl get crd maintenancewindows.freeze-operator.io -o yaml

# Test CR validation
kubectl apply --dry-run=server -f my-policy.yaml
```

## Getting Help

If issues persist:

1. **Gather diagnostic info:**

```bash
# Operator logs
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager \
  --tail=100 > operator-logs.txt

# Describe operator pod
kubectl describe pod -n kube-freeze-operator-system \
  -l control-plane=controller-manager > operator-pod.txt

# Policy status
kubectl get maintenancewindows,changefreezes,freezeexceptions \
  -o yaml > policies.yaml

# Webhook config
kubectl get validatingwebhookconfigurations \
  kube-freeze-operator-validating-webhook-configuration-workloads \
  -o yaml > webhook-config.yaml
```

2. **Check documentation:**

- [Architecture docs](architecture.md)
- [Usage guide](usage.md)
- [API reference](api-reference.md)

1. **Report issue:**

- Include diagnostic info above
- Kubernetes version
- Operator version
- Steps to reproduce

## Common Error Messages

### "admission webhook denied the request"

**Full message:**

```txt
Error from server: admission webhook "workloads.freeze-operator.io" denied the request:
Denied by ChangeFreeze/holiday-freeze: Production is frozen until 2026-12-27: 
Allowed after 2026-12-27T00:00:00Z
```

**Meaning:** Working as intended - freeze is active

**Action:** Wait for freeze to end or create FreezeException

### "no endpoints available for service"

**Full message:**

```txt
failed calling webhook: Post "https://...": no endpoints available for service
"kube-freeze-operator-webhook-service"
```

**Meaning:** Webhook service has no running pods

**Action:** Check operator deployment health

### "Internal error occurred: failed calling webhook"

**Meaning:** Webhook timeout or network issue

**Action:** Check webhook service, pods, and network policies
