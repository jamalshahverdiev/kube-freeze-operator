# Flux Integration

kube-freeze-operator can automatically suspend Flux Kustomizations and HelmReleases during active freeze windows and restore them after the freeze ends.

## How It Works

When a ChangeFreeze or MaintenanceWindow becomes active with GitOps enabled:

1. The operator finds Flux Kustomizations/HelmReleases matching your selectors
2. Saves the current `spec.suspend` value to an annotation
3. Sets `spec.suspend: true`
4. Marks the resource with `freeze-operator.io/managed=true`

When the freeze ends:

1. The operator finds resources it previously managed
2. Restores the original `spec.suspend` value from the saved annotation
3. Removes all freeze-operator annotations

This prevents Flux from repeatedly attempting reconciliation during a freeze.

## Supported Resources

| Resource | API Group | Version |
|---|---|---|
| Kustomization | `kustomize.toolkit.fluxcd.io` | `v1` |
| HelmRelease | `helm.toolkit.fluxcd.io` | `v2` |

## Annotations

| Annotation | Description |
|---|---|
| `freeze-operator.io/managed` | `"true"` while the operator controls this resource |
| `freeze-operator.io/managed-by-policy` | Name of the ChangeFreeze/MaintenanceWindow that suspended it |
| `freeze-operator.io/original-suspend` | Original `spec.suspend` value (`"true"` or `"false"`) |

## Configuration

Add the `behavior.gitops` block to your ChangeFreeze or MaintenanceWindow:

```yaml
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: year-end-freeze
spec:
  startTime: "2026-12-24T18:00:00Z"
  endTime: "2027-01-02T09:00:00Z"
  target:
    namespaceSelector:
      matchLabels:
        env: production
    kinds:
      - Deployment
  rules:
    deny:
      - ROLL_OUT
  behavior:
    gitops:
      enabled: true
      providers:
        - flux
      flux:
        namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: flux-system
        kustomizationSelector:
          matchLabels:
            env: production
        helmReleaseSelector:
          matchLabels:
            env: production
```

### Fields

| Field | Required | Description |
|---|---|---|
| `enabled` | Yes | Must be `true` to activate GitOps integration |
| `providers` | Yes | Include `"flux"` |
| `flux.kustomizationSelector` | No | Label selector for Kustomizations. If omitted, no Kustomizations are targeted |
| `flux.helmReleaseSelector` | No | Label selector for HelmReleases. If omitted, no HelmReleases are targeted |
| `flux.namespaceSelector` | No | Label selector for namespaces. If omitted, all namespaces are searched |

**Note:** At least one of `kustomizationSelector` or `helmReleaseSelector` should be set — otherwise no Flux resources will be managed.

## Already-Suspended Resources

If a Kustomization or HelmRelease was already `suspend: true` before the freeze:

- The operator still marks it as managed and saves `original-suspend: "true"`
- When the freeze ends, it restores `suspend: true` (not false)
- This preserves the user's original intent

## Conflict Protection

- The operator only restores resources it originally suspended (checked via `freeze-operator.io/managed-by-policy`)
- If a different policy suspended a resource, another policy won't restore it
- If Flux CRDs are not installed, the operator logs a debug message and skips — no errors

## Using Both ArgoCD and Flux

You can target both providers in a single policy:

```yaml
behavior:
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

## Webhook Noise Reduction

When Flux's kustomize-controller or helm-controller attempts changes during freeze, the webhook returns a short message:

```
[freeze-operator] sync blocked by <policy-name> until <end-time>
```

This keeps Flux controller logs clean compared to the full deny message shown to human users.

## RBAC

The operator needs permissions on Flux resources. These are generated from kubebuilder markers:

```yaml
- apiGroups: ["kustomize.toolkit.fluxcd.io"]
  resources: ["kustomizations"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["helm.toolkit.fluxcd.io"]
  resources: ["helmreleases"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

## Troubleshooting

### Resource not suspended during freeze

Check the operator logs:

```bash
kubectl logs -n kube-freeze-operator-system deployment/kube-freeze-operator-controller-manager -c manager | grep -i flux
```

Common causes:
- Resource labels don't match selector
- Resource is in a namespace not matching `namespaceSelector`
- `providers` list doesn't include `"flux"`
- Neither `kustomizationSelector` nor `helmReleaseSelector` is set

### Resource not restored after freeze ended

Check if the managed annotation is still present:

```bash
kubectl get kustomization <name> -n flux-system -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed-by-policy}'
```

If it shows a different policy name, that policy must end first.
