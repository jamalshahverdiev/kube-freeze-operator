*# kube-freeze-operator — Technical TODO Plan (v1.0)

> Goal v1.0: provide a clear and reliable Change Freeze / Maintenance Windows mechanism:
> - CRD: MaintenanceWindow, ChangeFreeze, FreezeException
> - Enforcement: Validating Admission Webhook (Deployments/StatefulSets/DaemonSets/CronJobs)
> - Controller: suspend/un-suspend CronJobs based on active policies
> - UX: clear deny messages + Events/metrics minimum
> - Helm chart + documentation + CI

---

## 0) Project bootstrap

- [ ] Choose final name and naming (repo/chart/image/namespace)
  - [ ] Repo: `kube-freeze-operator`
  - [ ] Namespace: `kube-freeze-operator`
  - [ ] Labels/annotations prefix: `freeze-operator.io/*`
- [ ] Define licensing and OSS metadata
  - [ ] LICENSE (Apache-2.0 or MIT)
  - [ ] CODE_OF_CONDUCT.md
  - [ ] CONTRIBUTING.md
  - [ ] SECURITY.md
- [ ] Repository structure
  - [ ] `/cmd/` (manager/webhook)
  - [ ] `/api/` (CRD types)
  - [ ] `/internal/` (policy engine, utils)
  - [ ] `/config/` (rbac, manager, webhook, crd, samples)
  - [ ] `/charts/` (Helm chart)
  - [ ] `/docs/` (architecture, how-to)
  - [ ] `/examples/` (real YAML scenarios)
- [ ] Define versions and release process
  - [ ] SemVer: `v1.0.0` = first stable MVP
  - [ ] Release artifacts: container image + helm chart + manifests

---

## 1) Specification (before code)

- [ ] Define v1.0 scope and non-goals (in README + docs/spec.md)
- [ ] Define list of supported resources v1.0:
  - [ ] Deployments
  - [ ] StatefulSets
  - [ ] DaemonSets
  - [ ] CronJobs
- [ ] Define operations (Actions) and determination rules:
  - [ ] CREATE / DELETE (by admission operation)
  - [ ] UPDATE → classification:
    - [ ] ROLL_OUT (PodTemplate change: `spec.template`)
    - [ ] SCALE (change `spec.replicas`)
    - [ ] CronJob mutation (schedule/jobTemplate etc.) — decide how to map (to ROLL_OUT or separate flag)
- [ ] Define policy priorities (deterministically):
  - [ ] First determine "is there an active deny"
  - [ ] Then apply Exception as override only for allowed operations
- [ ] Decide window model (for clarity):
  - [ ] v1.0: `mode = DenyOutsideWindows` for MaintenanceWindow (deny outside windows)
  - [ ] ChangeFreeze always "deny in range"
- [ ] Define UX deny-messages (template):
  - [ ] policy name + type
  - [ ] reason
  - [ ] when it will be allowed (next allowed window / endTime)
  - [ ] which operations are denied/allowed
  - [ ] docsURL + contact

---

## 2) CRD Design (API contracts)

### 2.1 MaintenanceWindow CRD
- [ ] Define `spec` fields:
  - [ ] `timezone` (IANA)
  - [ ] `mode` (v1.0: `DenyOutsideWindows`)
  - [ ] `windows[]`:
    - [ ] `name`
    - [ ] `schedule` (cron) — agree on format
    - [ ] `duration` (e.g., `6h`)
  - [ ] `target`:
    - [ ] `namespaceSelector`
    - [ ] `objectSelector`
    - [ ] `kinds[]` (Deployment/StatefulSet/DaemonSet/CronJob)
  - [ ] `rules`:
    - [ ] `deny[]` (ROLL_OUT/SCALE/CREATE/DELETE)
  - [ ] `behavior`:
    - [ ] `suspendCronJobs` (bool)
  - [ ] `message`:
    - [ ] `reason`
    - [ ] `docsURL`
    - [ ] `contact`
- [ ] Define `status` fields:
  - [ ] `active` (bool)
  - [ ] `activeWindow` (name, start/end)
  - [ ] `nextWindow` (start/end)
  - [ ] `observedGeneration`
  - [ ] conditions (Ready/Degraded/InvalidSchedule)

### 2.2 ChangeFreeze CRD
- [ ] Define `spec` fields:
  - [ ] `startTime`, `endTime` (RFC3339)
  - [ ] `timezone` (optional; if RFC3339 contains offset, can be omitted)
  - [ ] `target` (selectors)
  - [ ] `rules.deny[]`
  - [ ] `behavior.suspendCronJobs`
  - [ ] `message.*`
- [ ] Define `status`:
  - [ ] `active` (bool)
  - [ ] `timeRemaining`
  - [ ] conditions

### 2.3 FreezeException CRD
- [ ] Define `spec` fields:
  - [ ] `activeFrom`, `activeTo` (or `duration`)
  - [ ] `target` (selectors)
  - [ ] `allow[]` (ROLL_OUT/SCALE/CREATE/DELETE)
  - [ ] `constraints`:
    - [ ] `requireLabels` (map)
    - [ ] `allowedUsers` (list) — optional v1.0?
    - [ ] `allowedGroups` (list) — optional v1.0?
  - [ ] `reason`
  - [ ] `ticketURL`
  - [ ] `approvedBy` (optional)
- [ ] Define `status`:
  - [ ] `active` (bool)
  - [ ] conditions (Valid/Expired)

### 2.4 API conventions
- [ ] Define schema validations:
  - [ ] required fields (timezone, windows, start/end, etc.)
  - [ ] endTime > startTime
  - [ ] windows.duration > 0
  - [ ] deny/allow not empty when required
- [ ] Decide API grouping:
  - [ ] `freeze-operator.io/v1alpha1` for v1.0
- [ ] Add `config/samples/` for each CRD (at least 2 scenarios)

---

## 3) Policy Engine (decision computation logic)

- [ ] Design internal "policy evaluator":
  - [ ] Input: (now, resource meta, kind, namespace labels, object labels, operation, old/new diff)
  - [ ] Output: Allow/Deny + matchedPolicy + reason + nextAllowedTime
- [ ] Implement computation steps:
  - [ ] Find matching ChangeFreeze (by selectors)
  - [ ] Find matching MaintenanceWindow
  - [ ] Determine active deny state:
    - [ ] ChangeFreeze active? → deny state
    - [ ] MaintenanceWindow mode DenyOutsideWindows:
      - [ ] if currently not in window → deny state
      - [ ] if currently in window → allow state (if no ChangeFreeze)
  - [ ] If deny state active:
    - [ ] Check FreezeException (by selectors + active time)
    - [ ] Check constraints (labels/users/groups if enabled)
    - [ ] If exception allows specific operation → allow
    - [ ] Otherwise deny
- [ ] Precise diff logic:
  - [ ] ROLL_OUT detection for workloads
  - [ ] SCALE detection for deployments/statefulsets
  - [ ] CronJob mutation mapping (confirm)
- [ ] Unit tests policy engine (table tests):
  - [ ] “prod deny outside window”
  - [ ] “active changefreeze overrides window”
  - [ ] “exception allows rollout but not delete”
  - [ ] “label constraint required”
  - [ ] timezone correctness (DST cases at least basic)

---

## 4) Validating Admission Webhook (enforcement)

- [ ] Choose objects to intercept:
  - [ ] deployments.apps
  - [ ] statefulsets.apps
  - [ ] daemonsets.apps
  - [ ] cronjobs.batch
- [ ] Define operations:
  - [ ] CREATE, UPDATE, DELETE
- [ ] Implement handler:
  - [ ] Extract userInfo (username, groups)
  - [ ] Determine operation type (ROLL_OUT/SCALE/…)
  - [ ] Call policy engine
  - [ ] Format deny message by UX template
- [ ] Add "friendly" details:
  - [ ] policy type/name
  - [ ] next allowed window or freeze end time
  - [ ] docsURL/contact if present
- [ ] Decide failurePolicy:
  - [ ] configurable (chart values):
    - [ ] prod default: Fail
    - [ ] dev default: Ignore
- [ ] Webhook metrics:
  - [ ] allowed_total/denied_total + labels
- [ ] Logs:
  - [ ] structured logging: policy, namespace, kind, op, user, decision

---

## 5) CronJob Controller (suspend/un-suspend)

- [ ] Define "managed" behavior:
  - [ ] If freeze active and `suspendCronJobs=true`:
    - [ ] when `spec.suspend=false` → set true + annotate managed + store original
  - [ ] When freeze not active:
    - [ ] restore original value only for managed CronJobs
    - [ ] remove managed-annotations
- [ ] Define annotations:
  - [ ] `freeze-operator.io/managed: "true"`
  - [ ] `freeze-operator.io/original-suspend: "false|true"`
  - [ ] `freeze-operator.io/managed-by-policy: "<policyRef>"`
- [ ] Avoid conflicts:
  - [ ] if user manually changed suspend during managed — define rule (e.g., "don't touch if original absent")
- [ ] Load considerations:
  - [ ] avoid full cluster listing without need
  - [ ] use selectors and indexes (by namespace label possibly via cache)
- [ ] Unit/integration tests:
  - [ ] freeze on/off transition
  - [ ] CronJob already suspended (don't overwrite original)
  - [ ] multiple policies → choose correct (by priority)

---

## 6) Status + Reconciliation Observability

- [ ] Update status for CRD:
  - [ ] `active`, `nextWindow`, conditions
- [ ] Events:
  - [ ] `FreezeActivated`, `FreezeDeactivated`
  - [ ] `CronJobSuspended`, `CronJobResumed`
  - [ ] `WebhookDenied` (optional, careful with noise)
- [ ] Prometheus metrics (MVP):
  - [ ] `freeze_active{policy,kind}`
  - [ ] `freeze_webhook_denied_total{policy,ns,kind,action}`
  - [ ] `freeze_cronjob_suspended_total{policy,ns}`
- [ ] Health endpoints:
  - [ ] liveness/readiness for manager

---

## 7) Security / RBAC

- [ ] RBAC for operator:
  - [ ] get/list/watch required resources
  - [ ] patch CronJobs (and only them)
  - [ ] update status for CRDs
  - [ ] events create
- [ ] RBAC recommendation for users:
  - [ ] restrict `FreezeException` creation to separate group (SRE/Platform)
- [ ] Webhook TLS:
  - [ ] cert-manager integration (optional) or self-signed via helm hooks
- [ ] Consider "break-glass" scenario:
  - [ ] disable webhook via chart value / annotation (operational approach)

---

## 8) Packaging & Delivery

### 8.1 Container image
- [ ] Multi-arch build (amd64/arm64) — optional v1.0
- [ ] Minimal base image (distroless/alpine)
- [ ] SBOM/signing — optional v1.0

### 8.2 Helm chart
- [ ] Chart scaffold:
  - [ ] Deployment, Service, ServiceAccount, RBAC
  - [ ] Webhook configurations
  - [ ] CRDs (install/upgrade strategy)
- [ ] Values:
  - [ ] failurePolicy (Fail/Ignore)
  - [ ] replicas/resources
  - [ ] metrics enable
  - [ ] log level
  - [ ] namespace install
- [ ] Post-install notes:
  - [ ] how to apply sample policies

### 8.3 Manifests (kustomize)
- [ ] `/config/default` for "kubectl apply" install without Helm (optional)

---

## 9) CI/CD (GitLab)

- [ ] Pipeline stages:
  - [ ] lint (go fmt/vet)
  - [ ] unit tests
  - [ ] build image
  - [ ] helm lint
  - [ ] e2e (kind cluster) — optional, but highly desirable
- [ ] Release flow:
  - [ ] tag → build/push image
  - [ ] package/publish helm chart (GitLab package registry or chart repo)
- [ ] Automatic CRD manifests generation (control to keep always up-to-date)

---

## 10) Testing Strategy

### 10.1 Unit tests (mandatory)
- [ ] Policy engine (all main cases)
- [ ] Diff detection (rollout/scale)
- [ ] Timezone/window evaluation

### 10.2 Integration tests (desirable)
- [ ] envtest (controller-runtime) for webhook + controller logic
- [ ] kind e2e:
  - [ ] install operator
  - [ ] apply policy
  - [ ] try to update deployment → expect deny
  - [ ] create exception → expect allow (only for allowed action)
  - [ ] cronjob suspend/resume correctly

### 10.3 Performance sanity
- [ ] check behavior with 1000+ namespaces (without quadratic growth of listings)

---

## 11) Documentation

- [ ] README:
  - [ ] What/Why
  - [ ] Quickstart (install + sample policy)
  - [ ] Concepts (Freeze/Maintenance/Exception)
  - [ ] Supported resources v1.0
- [ ] docs/spec.md:
  - [ ] formal rules, priorities
- [ ] docs/usage.md:
  - [ ] examples for prod/stage
  - [ ] how to make hotfix exception
- [ ] docs/troubleshooting.md:
  - [ ] "why ArgoCD complains"
  - [ ] "how to know which policy is active"
  - [ ] "how to temporarily disable enforcement"
- [ ] docs/security.md:
  - [ ] RBAC recommendations
  - [ ] exception approval process
- [ ] Examples:
  - [ ] `examples/prod-night-window.yaml`
  - [ ] `examples/holiday-freeze.yaml`
  - [ ] `examples/hotfix-exception.yaml`

---

## 12) v1.0 Release Checklist

- [ ] All CRDs have schema validations and samples
- [ ] Webhook works stably on main objects
- [ ] CronJob suspend/resume without side effects
- [ ] Helm chart installs "in one step"
- [ ] Minimum metrics + clear deny messages
- [ ] Documentation: quickstart + troubleshooting
- [ ] Tag `v1.0.0` + release notes

---

# Roadmap (after v1.0)

## v2.0 — GitOps-friendly pause
- [x] ArgoCD Application pause/unpause (optional)
- [ ] Flux suspend/resume
- [ ] Reduce "noise" in GitOps on deny

## v3.0 — CI helper API
- [ ] REST endpoint `can-i-deploy?ns=&kind=&name=...`
- [ ] Ready-made templates for GitLab CI / GitHub Actions

## v4.0 — More targets & policies
- [ ] Jobs support
- [ ] More flexible window modes (deny windows)
- [ ] Global policy / multi-cluster strategy

## v5.0 — Better auditing
- [ ] CRD `FreezeEvent` (optional)
- [ ] integration with external audit sink (webhook/log exporter)
*