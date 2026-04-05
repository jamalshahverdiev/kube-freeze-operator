# ArgoCD Integration

kube-freeze-operator can automatically pause ArgoCD Application auto-sync during active freeze windows and restore it after the freeze ends.

## How It Works

When a ChangeFreeze or MaintenanceWindow becomes active with GitOps enabled:

1. The operator finds ArgoCD Applications matching your selectors
2. Saves the current `spec.syncPolicy.automated` value to an annotation
3. Removes `spec.syncPolicy.automated` to disable auto-sync
4. Marks the Application with `freeze-operator.io/managed=true`

When the freeze ends:

1. The operator finds Applications it previously managed
2. Restores the original `spec.syncPolicy.automated` from the saved annotation
3. Removes all freeze-operator annotations

This prevents ArgoCD from repeatedly attempting (and failing) to sync during a freeze, keeping the ArgoCD UI clean.

## Annotations

| Annotation | Description |
|---|---|
| `freeze-operator.io/managed` | `"true"` while the operator controls this Application |
| `freeze-operator.io/managed-by-policy` | Name of the ChangeFreeze/MaintenanceWindow that paused it |
| `freeze-operator.io/original-autosync` | JSON of the original `spec.syncPolicy.automated` value, or `"null"` if auto-sync was already disabled |

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
        - argocd
      argocd:
        namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: argocd
        applicationSelector:
          matchLabels:
            env: production
        pauseMode: DisableAutoSync
```

### Fields

| Field | Required | Description |
|---|---|---|
| `enabled` | Yes | Must be `true` to activate GitOps integration |
| `providers` | Yes | Include `"argocd"` |
| `argocd.applicationSelector` | No | Label selector for Applications. If omitted, all Applications in matching namespaces are targeted |
| `argocd.namespaceSelector` | No | Label selector for namespaces where Applications live. If omitted, all namespaces are searched |
| `argocd.pauseMode` | No | Defaults to `DisableAutoSync` |

## Selector Filtering

The operator applies **both** selectors:

- `namespaceSelector` — which namespaces to search for Applications
- `applicationSelector` — which Applications within those namespaces to manage

Applications that don't match the `applicationSelector` are left untouched, even if they're in a matching namespace. This lets you freeze production apps while leaving staging apps syncing.

## Conflict Protection

- The operator only restores Applications it originally paused (checked via `freeze-operator.io/managed-by-policy`)
- If a different ChangeFreeze paused an Application, another policy won't restore it
- If the ArgoCD CRD is not installed, the operator logs a debug message and skips — no errors

## Webhook Noise Reduction

When ArgoCD's application-controller attempts a sync during freeze, the webhook returns a short message:

```
[freeze-operator] sync blocked by <policy-name> until <end-time>
```

Instead of the full deny message shown to human users, this keeps ArgoCD logs clean.

## RBAC

The operator needs permissions on ArgoCD Application resources. These are generated from kubebuilder markers:

```yaml
- apiGroups: ["argoproj.io"]
  resources: ["applications"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

## Troubleshooting

### Application still shows auto-sync after freeze started

Check the operator logs:

```bash
kubectl logs -n kube-freeze-operator-system deployment/kube-freeze-operator-controller-manager -c manager | grep -i argocd
```

Common causes:
- Application labels don't match `applicationSelector`
- Application is in a namespace not matching `namespaceSelector`
- `providers` list doesn't include `"argocd"`

### Application not restored after freeze ended

Check if the managed annotation is still present:

```bash
kubectl get application <name> -n argocd -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed-by-policy}'
```

If it shows a different policy name, that policy must end first.
