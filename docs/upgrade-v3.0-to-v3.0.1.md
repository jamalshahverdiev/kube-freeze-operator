# Upgrade Guide: v3.0 to v3.0.1

## What's New in v3.0.1

v3.0.1 adds **TokenReview authentication** for the CI Helper API, allowing you to secure the API with Kubernetes ServiceAccount tokens.

### New Features

| Feature | Description |
|---|---|
| TokenReview Authentication | Validate Bearer tokens via Kubernetes TokenReview API |
| `--api-auth-mode` flag | Choose between `none` (default) and `token` authentication |
| RBAC for TokenReview | Automatic ClusterRole/Binding for `tokenreviews` API |

### New Flags

| Flag | Default | Description |
|---|---|---|
| `--api-auth-mode` | `none` | API authentication mode: `none` or `token` |

### Authentication Behavior

| Mode | `/healthz` | `/v1/evaluate` |
|---|---|---|
| `none` | Open | Open |
| `token` | Open (always) | Requires `Authorization: Bearer <token>` |

## Backward Compatibility

**v3.0.1 is fully backward compatible with v3.0.** The default `--api-auth-mode=none` preserves the existing behavior. All existing CI pipelines continue to work without changes.

## Upgrade Steps

### 1. Update the image

```bash
export IMG=jamalshahverdiev/kube-freeze-operator:v3.0.1
make deploy IMG=$IMG
```

Or with Helm:

```bash
helm upgrade kube-freeze-operator ./dist/chart \
  --namespace kube-freeze-operator-system \
  --set image.tag=v3.0.1
```

### 2. (Optional) Enable token authentication

#### With Kustomize / make deploy

Edit `config/manager/manager.yaml`:

```yaml
args:
  - --leader-elect
  - --health-probe-bind-address=:8081
  - --api-bind-address=:8082
  - --api-auth-mode=token    # change from "none" to "token"
```

Then redeploy:

```bash
make deploy IMG=$IMG
```

#### With Helm

```yaml
# values.yaml
api:
  enable: true
  port: 8082
  authMode: token
```

```bash
helm upgrade kube-freeze-operator ./dist/chart \
  --namespace kube-freeze-operator-system \
  --set api.authMode=token
```

### 3. Verify authentication is active

```bash
kubectl logs -n kube-freeze-operator-system \
  deployment/kube-freeze-operator-controller-manager -c manager \
  | grep "authentication"
# Expected: INFO api API server authentication enabled {"mode": "token"}
```

### 4. Create ServiceAccount for CI pipelines

```bash
# Create a dedicated SA
kubectl create serviceaccount freeze-ci-client -n gitlab-runners

# Get a token (valid for 1 hour)
TOKEN=$(kubectl create token freeze-ci-client -n gitlab-runners --duration=1h)

# Test
curl -sf http://kube-freeze-api.kube-freeze-operator-system:8082/v1/evaluate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"namespace":"prod","kind":"Deployment","action":"ROLL_OUT"}'
```

For CI runners with auto-mounted SA tokens (pods running in Kubernetes):

```bash
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
curl -sf "$FREEZE_API_URL/v1/evaluate" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"namespace":"prod","kind":"Deployment","action":"ROLL_OUT"}'
```

### 5. Update GitLab CI template (if using token auth)

```yaml
# .gitlab-ci.yml
freeze-check:
  stage: check
  extends: .freeze-check
  variables:
    FREEZE_API_URL: http://kube-freeze-api.kube-freeze-operator-system:8082
    FREEZE_NAMESPACE: prod
  script:
    - TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
    - |
      RESULT=$(curl -sf "$FREEZE_API_URL/v1/evaluate" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $TOKEN" \
        -d "{\"namespace\":\"$FREEZE_NAMESPACE\",\"kind\":\"Deployment\",\"action\":\"ROLL_OUT\"}")
      ALLOW=$(echo "$RESULT" | jq -r '.allow')
      if [ "$ALLOW" != "true" ]; then
        echo "Deployment blocked: $(echo $RESULT | jq -r '.reason')"
        exit 1
      fi
```

### 6. Verify healthz is still accessible without token

```bash
# healthz is always open, regardless of auth mode
curl -sf http://kube-freeze-api.kube-freeze-operator-system:8082/healthz
# Expected: ok
```

## RBAC Changes

v3.0.1 adds the following RBAC resources (only active when `--api-auth-mode=token`):

- **ClusterRole** `kube-freeze-operator-api-token-review-role`: Allows `create` on `tokenreviews`
- **ClusterRoleBinding** `kube-freeze-operator-api-token-review-rolebinding`: Binds the role to the controller-manager ServiceAccount

These are automatically deployed with `make deploy` or Helm (when `api.authMode: token`).

## No Breaking Changes

- Default auth mode is `none` — existing pipelines work without modification
- `/healthz` endpoint remains unauthenticated in all modes
- All CRD schemas are unchanged
- All webhook behavior is unchanged
- All GitOps behavior is unchanged
