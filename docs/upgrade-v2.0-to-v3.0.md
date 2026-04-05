# Upgrade Guide: v2.0 to v3.0

## What's New in v3.0

v3.0 adds a **CI Helper API** — an HTTP endpoint that CI/CD pipelines can query to check whether a deployment is currently allowed before proceeding.

### New Features

| Feature | Description |
|---|---|
| CI Helper API | `POST/GET /v1/evaluate` endpoint for freeze checks |
| GitLab CI template | Ready-to-use `.freeze-check` job in `ci/gitlab/freeze-check.yml` |
| API metrics | `freeze_operator_api_requests_total`, `freeze_operator_api_latency_seconds`, `freeze_operator_api_errors_total` |
| API health check | `GET /healthz` on the API port |
| Kubernetes Service | Dedicated Service for API access from CI runners |

### New Flags

| Flag | Default | Description |
|---|---|---|
| `--api-bind-address` | `:8082` | Address for the CI Helper API. Set to `0` to disable. |

### New Metrics

| Metric | Type | Labels |
|---|---|---|
| `freeze_operator_api_requests_total` | counter | `decision`, `namespace`, `kind`, `action` |
| `freeze_operator_api_latency_seconds` | histogram | — |
| `freeze_operator_api_errors_total` | counter | `error_type` |

## Backward Compatibility

**v3.0 is fully backward compatible with v1.0 and v2.0.** All existing ChangeFreeze, MaintenanceWindow, FreezeException, and GitOps behavior works without changes.

The API server is enabled by default on port `:8082`. To disable it:

```yaml
args:
  - --api-bind-address=0
```

## Upgrade Steps

### 1. Update the image

```bash
export IMG=jamalshahverdiev/kube-freeze-operator:v3.0.0
make deploy IMG=$IMG
```

Or update your deployment manifest:

```yaml
image: jamalshahverdiev/kube-freeze-operator:v3.0.0
```

### 2. Verify the API is running

```bash
kubectl logs -n kube-freeze-operator-system deploy/kube-freeze-operator-controller-manager -c manager | grep "starting API server"
# Expected: INFO api starting API server {"addr": ":8082"}
```

### 3. (Optional) Verify API access

```bash
kubectl port-forward -n kube-freeze-operator-system svc/kube-freeze-operator-controller-manager-api 8082:8082 &
curl -s http://localhost:8082/healthz
# Expected: ok
```

### 4. (Optional) Restrict API access with NetworkPolicy

If you want to limit which pods can reach the API:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: freeze-api-access
  namespace: kube-freeze-operator-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              freeze-api-access: "true"
      ports:
        - port: 8082
          protocol: TCP
```

Then label namespaces that need API access:

```bash
kubectl label ns gitlab-runners freeze-api-access=true
```

### 5. (Optional) Set up GitLab CI freeze check

Add to your `.gitlab-ci.yml`:

```yaml
include:
  - remote: 'https://raw.githubusercontent.com/<org>/kube-freeze-operator/v3.0.0/ci/gitlab/freeze-check.yml'

freeze-check:
  stage: check
  extends: .freeze-check
  variables:
    FREEZE_API_URL: http://kube-freeze-operator-controller-manager-api.kube-freeze-operator-system:8082
    FREEZE_NAMESPACE: prod
```

## No Breaking Changes

- All CRD schemas are unchanged
- All webhook behavior is unchanged
- All GitOps pause/resume behavior is unchanged
- The API is additive — it only reads existing policy state
