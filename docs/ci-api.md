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
