# kube-freeze-operator Helm Chart

Kubernetes operator for enforcing change freezes and maintenance windows.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- cert-manager v1.16+ (for webhook certificates)

## Installing the Chart

### Install cert-manager first

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.0/cert-manager.yaml
```

Wait for cert-manager to be ready:

```bash
kubectl wait --for=condition=Available --timeout=300s deployment -n cert-manager --all
```

### Install the operator

```bash
# Add the Helm repository (if published)
helm repo add kube-freeze-operator https://jamalshahverdiev.github.io/kube-freeze-operator
helm repo update

# Install the chart
helm install kube-freeze-operator kube-freeze-operator/kube-freeze-operator \
  --namespace kube-freeze-operator-system \
  --create-namespace
```

Or install from local chart:

```bash
helm install kube-freeze-operator ./dist/chart \
  --namespace kube-freeze-operator-system \
  --create-namespace
```

## Uninstalling the Chart

```bash
helm uninstall kube-freeze-operator -n kube-freeze-operator-system
```

**Note:** CRDs are kept by default. To remove them:

```bash
kubectl delete crd maintenancewindows.freeze-operator.io
kubectl delete crd changefreezes.freeze-operator.io
kubectl delete crd freezeexceptions.freeze-operator.io
```

## Configuration

### Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `manager.replicas` | Number of controller replicas | `1` |
| `manager.image.repository` | Controller image repository | `jamalshahverdiev/kube-freeze-operator` |
| `manager.image.tag` | Controller image tag | `v1.0.0` |
| `manager.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `manager.args` | Additional controller arguments | `["--leader-elect"]` |
| `manager.env` | Environment variables | `[]` |
| `manager.imagePullSecrets` | Image pull secrets | `[]` |
| `manager.resources.limits.cpu` | CPU limit | `500m` |
| `manager.resources.limits.memory` | Memory limit | `128Mi` |
| `manager.resources.requests.cpu` | CPU request | `10m` |
| `manager.resources.requests.memory` | Memory request | `64Mi` |
| `manager.affinity` | Pod affinity rules | `{}` |
| `manager.nodeSelector` | Node selector | `{}` |
| `manager.tolerations` | Pod tolerations | `[]` |
| `rbacHelpers.enable` | Install admin/editor/viewer roles | `false` |
| `crd.enable` | Install CRDs | `true` |
| `crd.keep` | Keep CRDs on uninstall | `true` |
| `metrics.enable` | Enable metrics endpoint | `true` |
| `metrics.port` | Metrics server port | `8443` |
| `certManager.enable` | Enable cert-manager integration | `true` |
| `webhook.enable` | Enable admission webhook | `true` |
| `webhook.port` | Webhook server port | `9443` |
| `prometheus.enable` | Enable Prometheus ServiceMonitor | `false` |

### Example: Custom Configuration

```yaml
# values-custom.yaml
manager:
  replicas: 2
  image:
    tag: v1.0.0
  resources:
    limits:
      cpu: 1000m
      memory: 256Mi
    requests:
      cpu: 50m
      memory: 128Mi
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                control-plane: controller-manager
            topologyKey: kubernetes.io/hostname
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule

prometheus:
  enable: true

rbacHelpers:
  enable: true
```

Install with custom values:

```bash
helm install kube-freeze-operator ./dist/chart \
  -f values-custom.yaml \
  --namespace kube-freeze-operator-system \
  --create-namespace
```

### Example: Production Configuration

```yaml
# values-prod.yaml
manager:
  replicas: 3
  image:
    tag: v1.0.0
    pullPolicy: Always
  args:
    - --leader-elect
    - --zap-log-level=info
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
    requests:
      cpu: 100m
      memory: 256Mi
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              control-plane: controller-manager
          topologyKey: kubernetes.io/hostname

metrics:
  enable: true

prometheus:
  enable: true

rbacHelpers:
  enable: true
```

## Upgrading

### To 1.0.0 from pre-release versions

```bash
helm upgrade kube-freeze-operator ./dist/chart \
  --namespace kube-freeze-operator-system \
  --reuse-values
```

## Troubleshooting

### Webhook certificate issues

If you see webhook errors like "x509: certificate signed by unknown authority":

```bash
# Check cert-manager is running
kubectl get pods -n cert-manager

# Check certificate
kubectl get certificate -n kube-freeze-operator-system
kubectl describe certificate -n kube-freeze-operator-system serving-cert

# Check webhook configuration
kubectl get validatingwebhookconfigurations | grep kube-freeze
```

### Pod not starting

```bash
# Check pod status
kubectl get pods -n kube-freeze-operator-system

# Check logs
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager

# Check events
kubectl get events -n kube-freeze-operator-system --sort-by='.lastTimestamp'
```

### CRDs not installed

```bash
# Verify CRDs
kubectl get crds | grep freeze-operator.io

# Manually install if needed
kubectl apply -f config/crd/bases/
```

## Additional Documentation

- [Usage Guide](../../docs/usage.md)
- [Architecture](../../docs/architecture.md)
- [API Reference](../../docs/api-reference.md)
- [Troubleshooting](../../docs/troubleshooting.md)

## License

Apache License 2.0. See [LICENSE](../../LICENSE) for details.
