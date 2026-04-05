# Upgrade Guide: v1.0 to v2.0

## What's New in v2.0

v2.0 adds **GitOps-friendly pause/resume** — the operator can automatically pause ArgoCD Applications and Flux Kustomizations/HelmReleases during active freeze windows.

### New Features

| Feature | Description |
|---|---|
| ArgoCD pause/restore | Disable `spec.syncPolicy.automated` during freeze, restore after |
| Flux suspend/restore | Set `spec.suspend: true` on Kustomizations and HelmReleases during freeze |
| HelmRelease support | Full support for `helm.toolkit.fluxcd.io/v2` HelmRelease resources |
| Webhook noise reduction | Short deny messages for ArgoCD/Flux service accounts |
| Graceful CRD absence | Operator works fine if ArgoCD/Flux CRDs are not installed |
| Selector filtering | Target specific Applications/Kustomizations/HelmReleases by label |

### New Status Fields

Both ChangeFreeze and MaintenanceWindow now expose:

- `status.gitopsPausedCount` — number of GitOps resources currently paused
- `status.gitopsLastReconcileTime` — last time the GitOps reconciler ran

## Backward Compatibility

**v2.0 is fully backward compatible with v1.0 policies.** Existing ChangeFreeze, MaintenanceWindow, and FreezeException resources work without any changes.

The new `behavior.gitops` block is entirely optional. If omitted, the operator behaves exactly as v1.0.

## Upgrade Steps

### 1. Update CRDs

The CRD schemas have new fields. Apply the updated CRDs before deploying the new operator:

```bash
make manifests
kubectl apply -k config/crd
```

### 2. Deploy the New Operator

```bash
export IMG=<registry>/kube-freeze-operator:v2.0.0
make docker-build docker-push IMG=$IMG
make deploy IMG=$IMG
```

### 3. Update RBAC (if needed)

v2.0 adds RBAC rules for ArgoCD and Flux resources. These are included in the generated `config/rbac/role.yaml` and applied automatically with `make deploy`.

New permissions added:

```yaml
# ArgoCD
- apiGroups: ["argoproj.io"]
  resources: ["applications"]
  verbs: ["get", "list", "watch", "update", "patch"]

# Flux Kustomization
- apiGroups: ["kustomize.toolkit.fluxcd.io"]
  resources: ["kustomizations"]
  verbs: ["get", "list", "watch", "update", "patch"]

# Flux HelmRelease
- apiGroups: ["helm.toolkit.fluxcd.io"]
  resources: ["helmreleases"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

If ArgoCD or Flux is not installed in your cluster, these permissions have no effect.

### 4. Enable GitOps Integration (Optional)

Add the `behavior.gitops` block to your existing policies:

```yaml
# Before (v1.0)
spec:
  behavior:
    suspendCronJobs: true

# After (v2.0) — add gitops block
spec:
  behavior:
    suspendCronJobs: true
    gitops:
      enabled: true
      providers:
        - argocd
        - flux
      argocd:
        applicationSelector:
          matchLabels:
            env: production
      flux:
        kustomizationSelector:
          matchLabels:
            env: production
        helmReleaseSelector:
          matchLabels:
            env: production
```

## Verification

After upgrading, verify:

```bash
# Check operator is running
kubectl get pods -n kube-freeze-operator-system

# Check CRDs are updated (should have behavior.gitops in spec)
kubectl explain changefreeze.spec.behavior.gitops

# Check status fields exist
kubectl get changefreeze <name> -o jsonpath='{.status.gitopsPausedCount}'
```

## Rollback

To roll back to v1.0:

1. Remove `behavior.gitops` blocks from all policies
2. Deploy the v1.0 operator image
3. CRD schema changes are additive — v1.0 operator ignores unknown fields

If GitOps resources were paused when you roll back, manually restore them:

```bash
# Restore ArgoCD Applications
kubectl get applications -n argocd -l freeze-operator.io/managed=true
# For each: remove annotations and re-enable autosync

# Restore Flux resources
kubectl get kustomizations -A -l freeze-operator.io/managed=true
# For each: set spec.suspend back to original value
```
