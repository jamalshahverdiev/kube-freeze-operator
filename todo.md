# kube-freeze-operator — Technical TODO Plan (v0.1)

> Цель v0.1: дать понятный и надежный механизм Change Freeze / Maintenance Windows:
> - CRD: MaintenanceWindow, ChangeFreeze, FreezeException
> - Enforcement: Validating Admission Webhook (Deployments/StatefulSets/DaemonSets/CronJobs)
> - Controller: suspend/un-suspend CronJobs по активным правилам
> - UX: понятные deny-сообщения + Events/metrics минимум
> - Helm chart + документация + CI

---

## 0) Project bootstrap

- [ ] Выбрать итоговое название и нейминг (repo/chart/image/namespace)
  - [ ] Repo: `kube-freeze-operator`
  - [ ] Namespace: `kube-freeze-operator`
  - [ ] Labels/annotations prefix: `freeze-operator.io/*`
- [ ] Определить лицензирование и метаданные OSS
  - [ ] LICENSE (Apache-2.0 или MIT)
  - [ ] CODE_OF_CONDUCT.md
  - [ ] CONTRIBUTING.md
  - [ ] SECURITY.md
- [ ] Структура репозитория
  - [ ] `/cmd/` (manager/webhook)
  - [ ] `/api/` (CRD types)
  - [ ] `/internal/` (policy engine, utils)
  - [ ] `/config/` (rbac, manager, webhook, crd, samples)
  - [ ] `/charts/` (Helm chart)
  - [ ] `/docs/` (архитектура, how-to)
  - [ ] `/examples/` (реальные YAML сценарии)
- [ ] Определить версии и релизный процесс
  - [ ] SemVer: `v0.1.0` = первый стабильный MVP
  - [ ] Release artifacts: container image + helm chart + manifests

---

## 1) Спецификация (до кода)

- [ ] Зафиксировать v0.1 scope и non-goals (в README + docs/spec.md)
- [ ] Зафиксировать список поддерживаемых ресурсов v0.1:
  - [ ] Deployments
  - [ ] StatefulSets
  - [ ] DaemonSets
  - [ ] CronJobs
- [ ] Зафиксировать операции (Actions) и правила определения:
  - [ ] CREATE / DELETE (по admission operation)
  - [ ] UPDATE → классификация:
    - [ ] ROLL_OUT (изменение PodTemplate: `spec.template`)
    - [ ] SCALE (изменение `spec.replicas`)
    - [ ] CronJob mutation (schedule/jobTemplate и т.п.) — решить как маппить (в ROLL_OUT или отдельный флаг)
- [ ] Зафиксировать приоритеты политик (детерминированно):
  - [ ] Сначала определить “есть ли активный запрет”
  - [ ] Затем применить Exception как override только для разрешенных операций
- [ ] Решить модель окон (для ясности):
  - [ ] v0.1: `mode = DenyOutsideWindows` для MaintenanceWindow (вне окон запрещено)
  - [ ] ChangeFreeze всегда “deny in range”
- [ ] Зафиксировать UX deny-сообщения (шаблон):
  - [ ] policy name + type
  - [ ] reason
  - [ ] когда будет разрешено (next allowed window / endTime)
  - [ ] какие операции запрещены/разрешены
  - [ ] docsURL + contact

---

## 2) CRD Design (API contracts)

### 2.1 MaintenanceWindow CRD
- [ ] Определить `spec` поля:
  - [ ] `timezone` (IANA)
  - [ ] `mode` (v0.1: `DenyOutsideWindows`)
  - [ ] `windows[]`:
    - [ ] `name`
    - [ ] `schedule` (cron) — договориться о формате
    - [ ] `duration` (например, `6h`)
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
- [ ] Определить `status` поля:
  - [ ] `active` (bool)
  - [ ] `activeWindow` (name, start/end)
  - [ ] `nextWindow` (start/end)
  - [ ] `observedGeneration`
  - [ ] conditions (Ready/Degraded/InvalidSchedule)

### 2.2 ChangeFreeze CRD
- [ ] Определить `spec` поля:
  - [ ] `startTime`, `endTime` (RFC3339)
  - [ ] `timezone` (опционально; если RFC3339 содержит offset, можно не требовать)
  - [ ] `target` (selectors)
  - [ ] `rules.deny[]`
  - [ ] `behavior.suspendCronJobs`
  - [ ] `message.*`
- [ ] Определить `status`:
  - [ ] `active` (bool)
  - [ ] `timeRemaining`
  - [ ] conditions

### 2.3 FreezeException CRD
- [ ] Определить `spec` поля:
  - [ ] `activeFrom`, `activeTo` (или `duration`)
  - [ ] `target` (selectors)
  - [ ] `allow[]` (ROLL_OUT/SCALE/CREATE/DELETE)
  - [ ] `constraints`:
    - [ ] `requireLabels` (map)
    - [ ] `allowedUsers` (list) — optional v0.1?
    - [ ] `allowedGroups` (list) — optional v0.1?
  - [ ] `reason`
  - [ ] `ticketURL`
  - [ ] `approvedBy` (optional)
- [ ] Определить `status`:
  - [ ] `active` (bool)
  - [ ] conditions (Valid/Expired)

### 2.4 API conventions
- [ ] Прописать валидации схемы:
  - [ ] обязательные поля (timezone, windows, start/end, etc.)
  - [ ] endTime > startTime
  - [ ] windows.duration > 0
  - [ ] deny/allow не пустые при необходимости
- [ ] Решить группировку API:
  - [ ] `freeze-operator.io/v1alpha1` для v0.1
- [ ] Добавить `config/samples/` для каждого CRD (минимум 2 сценария)

---

## 3) Policy Engine (логика вычисления решения)

- [ ] Спроектировать внутренний “policy evaluator”:
  - [ ] Вход: (now, resource meta, kind, namespace labels, object labels, operation, old/new diff)
  - [ ] Выход: Allow/Deny + matchedPolicy + reason + nextAllowedTime
- [ ] Реализовать шаги вычисления:
  - [ ] Найти совпадающие ChangeFreeze (по selectors)
  - [ ] Найти совпадающие MaintenanceWindow
  - [ ] Определить active deny state:
    - [ ] ChangeFreeze active? → deny state
    - [ ] MaintenanceWindow mode DenyOutsideWindows:
      - [ ] если сейчас не в окне → deny state
      - [ ] если сейчас в окне → allow state (если нет ChangeFreeze)
  - [ ] Если deny state активен:
    - [ ] Проверить FreezeException (по selectors + active time)
    - [ ] Проверить constraints (labels/users/groups если включено)
    - [ ] Если exception разрешает конкретную операцию → allow
    - [ ] Иначе deny
- [ ] Точная логика diff:
  - [ ] ROLL_OUT detection для workloads
  - [ ] SCALE detection для deployments/statefulsets
  - [ ] CronJob mutation mapping (утвердить)
- [ ] Unit-тесты policy engine (табличные тесты):
  - [ ] “prod deny outside window”
  - [ ] “active changefreeze overrides window”
  - [ ] “exception allows rollout but not delete”
  - [ ] “label constraint required”
  - [ ] timezone correctness (DST кейсы хотя бы базово)

---

## 4) Validating Admission Webhook (enforcement)

- [ ] Выбрать объекты для перехвата:
  - [ ] deployments.apps
  - [ ] statefulsets.apps
  - [ ] daemonsets.apps
  - [ ] cronjobs.batch
- [ ] Определить operations:
  - [ ] CREATE, UPDATE, DELETE
- [ ] Реализовать обработчик:
  - [ ] Извлечение userInfo (username, groups)
  - [ ] Определение operation type (ROLL_OUT/SCALE/…)
  - [ ] Вызов policy engine
  - [ ] Формирование deny message по шаблону UX
- [ ] Добавить “дружелюбные” details:
  - [ ] policy type/name
  - [ ] next allowed window or freeze end time
  - [ ] docsURL/contact if present
- [ ] Решить failurePolicy:
  - [ ] configurable (chart values):
    - [ ] prod default: Fail
    - [ ] dev default: Ignore
- [ ] Метрики webhook:
  - [ ] allowed_total/denied_total + labels
- [ ] Логи:
  - [ ] structured logging: policy, namespace, kind, op, user, decision

---

## 5) CronJob Controller (suspend/un-suspend)

- [ ] Определить поведение “managed”:
  - [ ] Если freeze активен и `suspendCronJobs=true`:
    - [ ] при `spec.suspend=false` → set true + annotate managed + store original
  - [ ] Когда freeze не активен:
    - [ ] вернуть исходное значение только тем CronJob, что managed
    - [ ] удалить managed-annotations
- [ ] Определить аннотации:
  - [ ] `freeze-operator.io/managed: "true"`
  - [ ] `freeze-operator.io/original-suspend: "false|true"`
  - [ ] `freeze-operator.io/managed-by-policy: "<policyRef>"`
- [ ] Избежать конфликтов:
  - [ ] если пользователь вручную изменил suspend во время managed — определить правило (например “не трогать, если original отсутствует”)
- [ ] Нагрузочные нюансы:
  - [ ] избегать полного листинга всего кластера без нужды
  - [ ] использовать selectors и индексы (по namespace label возможно через кэш)
- [ ] Unit/integration тесты:
  - [ ] переход freeze on/off
  - [ ] CronJob already suspended (не перезаписывать original)
  - [ ] несколько политик → выбрать корректную (по приоритету)

---

## 6) Status + Reconciliation Observability

- [ ] Обновление status для CRD:
  - [ ] `active`, `nextWindow`, conditions
- [ ] Events:
  - [ ] `FreezeActivated`, `FreezeDeactivated`
  - [ ] `CronJobSuspended`, `CronJobResumed`
  - [ ] `WebhookDenied` (опционально, осторожно с шумом)
- [ ] Prometheus metrics (MVP):
  - [ ] `freeze_active{policy,kind}`
  - [ ] `freeze_webhook_denied_total{policy,ns,kind,action}`
  - [ ] `freeze_cronjob_suspended_total{policy,ns}`
- [ ] Health endpoints:
  - [ ] liveness/readiness для manager

---

## 7) Security / RBAC

- [ ] RBAC для оператора:
  - [ ] get/list/watch нужных ресурсов
  - [ ] patch CronJobs (и только их)
  - [ ] update status для CRDs
  - [ ] events create
- [ ] RBAC рекомендация пользователям:
  - [ ] ограничить создание `FreezeException` отдельной группой (SRE/Platform)
- [ ] Webhook TLS:
  - [ ] cert-manager integration (опционально) или self-signed через helm hooks
- [ ] Подумать про “break-glass” сценарий:
  - [ ] disable webhook via chart value / annotation (операционный подход)

---

## 8) Packaging & Delivery

### 8.1 Container image
- [ ] Multi-arch build (amd64/arm64) — optional v0.1
- [ ] Minimal base image (distroless/alpine)
- [ ] SBOM/signing — optional v0.1

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
  - [ ] как применить sample policies

### 8.3 Manifests (kustomize)
- [ ] `/config/default` для “kubectl apply” установки без Helm (optional)

---

## 9) CI/CD (GitLab)

- [ ] Pipeline stages:
  - [ ] lint (go fmt/vet)
  - [ ] unit tests
  - [ ] build image
  - [ ] helm lint
  - [ ] e2e (kind cluster) — optional, но очень желательно
- [ ] Release flow:
  - [ ] tag → build/push image
  - [ ] package/publish helm chart (GitLab package registry или chart repo)
- [ ] Автоматическая генерация CRD manifests (контроль, чтобы всегда актуально)

---

## 10) Testing Strategy

### 10.1 Unit tests (обязательные)
- [ ] Policy engine (все основные кейсы)
- [ ] Diff detection (rollout/scale)
- [ ] Timezone/window evaluation

### 10.2 Integration tests (желательно)
- [ ] envtest (controller-runtime) для webhook + controller логики
- [ ] kind e2e:
  - [ ] установить operator
  - [ ] применить policy
  - [ ] попытаться обновить deployment → ожидаем deny
  - [ ] создать exception → ожидаем allow (только для разрешенного действия)
  - [ ] cronjob suspend/resume корректно

### 10.3 Performance sanity
- [ ] проверить поведение при 1000+ namespaces (без квадратичного роста листингов)

---

## 11) Documentation

- [ ] README:
  - [ ] What/Why
  - [ ] Quickstart (install + sample policy)
  - [ ] Concepts (Freeze/Maintenance/Exception)
  - [ ] Supported resources v0.1
- [ ] docs/spec.md:
  - [ ] формальные правила, приоритеты
- [ ] docs/usage.md:
  - [ ] примеры для prod/stage
  - [ ] как сделать hotfix exception
- [ ] docs/troubleshooting.md:
  - [ ] “почему ArgoCD ругается”
  - [ ] “как узнать какая policy активна”
  - [ ] “как временно отключить enforcement”
- [ ] docs/security.md:
  - [ ] RBAC recommendations
  - [ ] exception approval process
- [ ] Examples:
  - [ ] `examples/prod-night-window.yaml`
  - [ ] `examples/holiday-freeze.yaml`
  - [ ] `examples/hotfix-exception.yaml`

---

## 12) v0.1 Release Checklist

- [ ] Все CRDs имеют schema validations и samples
- [ ] Webhook стабильно работает на основных объектах
- [ ] CronJob suspend/resume без побочных эффектов
- [ ] Helm chart устанавливается “в один шаг”
- [ ] Минимум метрик + понятные deny messages
- [ ] Документация: quickstart + troubleshooting
- [ ] Tag `v0.1.0` + release notes

---

# Roadmap (после v0.1)

## v0.2 — GitOps-friendly pause
- [x] ArgoCD Application pause/unpause (опционально)
- [ ] Flux suspend/resume
- [ ] Снижение “шума” в GitOps при deny

## v0.3 — CI helper API
- [ ] REST endpoint `can-i-deploy?ns=&kind=&name=...`
- [ ] Готовые templates для GitLab CI / GitHub Actions

## v0.4 — More targets & policies
- [ ] Поддержка Jobs
- [ ] Более гибкие режимы окон (deny windows)
- [ ] Global policy / multi-cluster strategy

## v0.5 — Better auditing
- [ ] CRD `FreezeEvent` (опционально)
- [ ] интеграция с внешним audit sink (webhook/log exporter)
