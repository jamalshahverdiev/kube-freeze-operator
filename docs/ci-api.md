# CI Helper API

The CI Helper API provides an HTTP endpoint that CI/CD pipelines can call to check whether a deployment is allowed before proceeding.

## Overview

The API server runs inside the operator pod on port `8082` (configurable) and exposes `POST /v1/evaluate` and `GET /v1/evaluate` endpoints that evaluate the current freeze/maintenance window policies for a given namespace, kind, and action.

## Enabling the API

The API is enabled by default. Configure it with the `--api-bind-address` flag:

```yaml
# In manager deployment
args:
  - --api-bind-address=:8082   # default
  - --api-auth-mode=none       # "none" (default) or "token"
  # - --api-bind-address=0     # disable API
```

## Endpoints

### POST /v1/evaluate

Evaluate whether a deployment action is currently allowed.

**Request:**

```json
{
  "namespace": "prod",
  "kind": "Deployment",
  "action": "ROLL_OUT"
}
```

| Field       | Required | Values                                            |
|-------------|----------|---------------------------------------------------|
| `namespace` | yes      | Kubernetes namespace                               |
| `kind`      | yes      | `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`|
| `action`    | yes      | `CREATE`, `DELETE`, `ROLL_OUT`, `SCALE`            |
| `name`      | no       | Resource name (reserved for future use)            |

**Response (allowed):**

```json
{
  "allow": true,
  "evaluatedAt": "2026-01-28T12:00:00Z"
}
```

**Response (denied):**

```json
{
  "allow": false,
  "reason": "Blocked by ChangeFreeze cf-release (active until 2026-01-29T00:00:00Z)",
  "matchedPolicy": "cf-release",
  "policyKind": "ChangeFreeze",
  "freezeEndTime": "2026-01-29T00:00:00Z",
  "nextAllowedTime": "2026-01-29T00:00:00Z",
  "evaluatedAt": "2026-01-28T12:00:00Z"
}
```

### GET /v1/evaluate

Same as POST but with query parameters:

```
GET /v1/evaluate?namespace=prod&kind=Deployment&action=ROLL_OUT
```

### GET /healthz

Health check endpoint. Returns `200 OK` with body `ok`.

## Authentication

By default the API has no authentication (`--api-auth-mode=none`). In production, enable **TokenReview** authentication so that only requests with valid Kubernetes ServiceAccount tokens are accepted.

### Enabling Token Authentication

```yaml
# In manager deployment args
args:
  - --api-bind-address=:8082
  - --api-auth-mode=token   # "none" (default) or "token"
```

With Helm:

```yaml
# values.yaml
api:
  enable: true
  port: 8082
  authMode: token   # enables TokenReview authentication
```

### How It Works

1. Client sends `Authorization: Bearer <token>` header
2. Operator calls Kubernetes `TokenReview` API to validate the token
3. If token is valid — request proceeds to the evaluate handler
4. If token is missing or invalid — `401 Unauthorized`

The `/healthz` endpoint is **always** accessible without a token (for readiness probes).

### Creating a ServiceAccount for CI

```bash
# Create a dedicated SA for CI pipelines
kubectl create serviceaccount freeze-ci-client -n gitlab-runners

# Get a short-lived token (10 min)
TOKEN=$(kubectl create token freeze-ci-client -n gitlab-runners --duration=10m)

# Use in requests
curl -sf http://kube-freeze-api.kube-freeze-operator-system:8082/v1/evaluate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"namespace":"prod","kind":"Deployment","action":"ROLL_OUT"}'
```

For CI runners with mounted ServiceAccount tokens (e.g., pods in Kubernetes), use the auto-mounted token:

```bash
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
curl -sf "$FREEZE_API_URL/v1/evaluate" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"namespace\":\"$FREEZE_NAMESPACE\",\"kind\":\"Deployment\",\"action\":\"ROLL_OUT\"}"
```

### Auth Modes Summary

| Mode | Flag | Behavior |
|------|------|----------|
| `none` | `--api-auth-mode=none` | No authentication (default) |
| `token` | `--api-auth-mode=token` | Validates Bearer token via Kubernetes TokenReview |

### RBAC for TokenReview

When `--api-auth-mode=token` is enabled, the operator needs permission to create `tokenreviews`. This is automatically configured:

- Kustomize: `config/rbac/api_token_review_role.yaml`
- Helm: `templates/rbac/api-token-review.yaml` (conditional on `api.authMode: token`)

## Accessing the API from CI/CD

### Option 1: In-cluster Service

Create a Kubernetes Service that exposes the API port:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kube-freeze-api
  namespace: kube-freeze-operator-system
spec:
  selector:
    control-plane: controller-manager
  ports:
    - port: 8082
      targetPort: 8082
      name: api
```

Then call from CI jobs running in the same cluster:

```bash
curl -sf http://kube-freeze-api.kube-freeze-operator-system:8082/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{"namespace":"prod","kind":"Deployment","action":"ROLL_OUT"}'
```

### Option 2: Port-forward (development)

```bash
kubectl port-forward -n kube-freeze-operator-system deploy/kube-freeze-operator-controller-manager 8082:8082
curl -sf http://localhost:8082/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{"namespace":"prod","kind":"Deployment","action":"ROLL_OUT"}'
```

## GitLab CI Integration

See the [GitLab CI template](../ci/gitlab/freeze-check.yml) for a ready-to-use job.

**Quick setup:**

```yaml
# .gitlab-ci.yml
include:
  - local: 'ci/gitlab/freeze-check.yml'

stages:
  - check
  - deploy

freeze-check:
  stage: check
  extends: .freeze-check
  variables:
    FREEZE_API_URL: http://kube-freeze-api.kube-freeze-operator-system:8082
    FREEZE_NAMESPACE: prod

deploy:
  stage: deploy
  needs: [freeze-check]
  script:
    - kubectl apply -f manifests/
```

## Exit Codes for CI

| API Response  | CI Behavior                              |
|---------------|------------------------------------------|
| `allow: true` | Job succeeds (exit 0)                    |
| `allow: false`| Job fails (exit 1) — deployment blocked  |
| API error     | Depends on `FREEZE_FAIL_ON_ERROR` setting|
