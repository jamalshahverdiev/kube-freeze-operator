#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

IMG_DEFAULT="jamalshahverdiev/kube-freeze-operator:v1.0.4"
IMG="${IMG:-$IMG_DEFAULT}"

REDEPLOY="${REDEPLOY:-true}"

PROD_NS="${PROD_NS:-prod-freeze-test}"
DEV_NS="${DEV_NS:-dev-freeze-test}"

TEST_LABEL_KEY="${TEST_LABEL_KEY:-freeze-operator-test}"
TEST_LABEL_VALUE="${TEST_LABEL_VALUE:-true}"
TEST_SCOPE_KEY="${TEST_SCOPE_KEY:-freeze-test-scope}"
TEST_SCOPE_VALUE="${TEST_SCOPE_VALUE:-maintenancewindow}"

PROD_TIER_KEY="${PROD_TIER_KEY:-freeze-test-tier}"
PROD_TIER_VALUE="${PROD_TIER_VALUE:-deny}"
DEV_TIER_VALUE="${DEV_TIER_VALUE:-allow}"

banner() {
  printf "\n== %s ==\n" "$1"
}

ensure_namespace_deleted() {
  local ns="$1"
  if kubectl get ns "${ns}" >/dev/null 2>&1; then
    kubectl delete ns "${ns}" --wait=false >/dev/null 2>&1 || true
  fi

  # Namespace deletion can take time (finalizers, GC). Wait until it's actually gone.
  for _ in $(seq 1 90); do
    if ! kubectl get ns "${ns}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "ERROR: namespace ${ns} still exists after waiting for deletion" >&2
  kubectl get ns "${ns}" -o yaml | sed -n '1,80p' >&2 || true
  return 1
}

run_expect_allow() {
  local desc="$1"; shift
  echo "--- ${desc}"
  set +e
  local out
  if command -v timeout >/dev/null 2>&1; then
    out="$(timeout 25s "$@" 2>&1)"
  else
    out="$($@ 2>&1)"
  fi
  local rc=$?
  set -e
  echo "${out}" | sed -n '1,25p'
  if [ ${rc} -ne 0 ]; then
    echo "RESULT: FAIL (command failed, expected allow)"
    fail=1
  else
    echo "RESULT: PASS (allowed as expected)"
  fi
  echo
}

banner "Context"
kubectl config current-context
kubectl version --client --output=yaml | sed -n '1,40p' || true

if [ "${REDEPLOY}" = "true" ]; then
  banner "Redeploy (${IMG})"
  make deploy "IMG=${IMG}"
  kubectl -n kube-freeze-operator-system rollout status deployment/kube-freeze-operator-controller-manager --timeout=180s
  kubectl -n kube-freeze-operator-system get pods -o wide
fi

banner "Webhook plumbing"
kubectl -n kube-freeze-operator-system get svc kube-freeze-operator-webhook-service
kubectl -n kube-freeze-operator-system get endpoints kube-freeze-operator-webhook-service
kubectl -n kube-freeze-operator-system get issuer,certificate,secret
printf 'workloads caBundle prefix: '
kubectl get validatingwebhookconfiguration kube-freeze-operator-validating-webhook-configuration-workloads \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | head -c 16; echo
kubectl get validatingwebhookconfiguration kube-freeze-operator-validating-webhook-configuration-workloads -o yaml | \
  awk '/resources: \[/,/namespaceSelector:/{print}' | sed -n '1,120p'

banner "Cleanup existing test freeze policies (deterministic + safe)"
kubectl delete maintenancewindows.freeze-operator.io -l "${TEST_LABEL_KEY}=${TEST_LABEL_VALUE},${TEST_SCOPE_KEY}=${TEST_SCOPE_VALUE}" --ignore-not-found
kubectl delete changefreezes.freeze-operator.io -l "${TEST_LABEL_KEY}=${TEST_LABEL_VALUE},${TEST_SCOPE_KEY}=${TEST_SCOPE_VALUE}" --ignore-not-found
kubectl delete freezeexceptions.freeze-operator.io -l "${TEST_LABEL_KEY}=${TEST_LABEL_VALUE},${TEST_SCOPE_KEY}=${TEST_SCOPE_VALUE}" --ignore-not-found

fail=0

banner "Reset test namespaces (${PROD_NS}, ${DEV_NS})"
for ns in "${PROD_NS}" "${DEV_NS}"; do
  ensure_namespace_deleted "${ns}"
  kubectl create ns "${ns}"
done
kubectl label ns "${PROD_NS}" \
  "${TEST_LABEL_KEY}=${TEST_LABEL_VALUE}" \
  "${TEST_SCOPE_KEY}=${TEST_SCOPE_VALUE}" \
  "${PROD_TIER_KEY}=${PROD_TIER_VALUE}" \
  --overwrite
kubectl label ns "${DEV_NS}" \
  "${TEST_LABEL_KEY}=${TEST_LABEL_VALUE}" \
  "${TEST_SCOPE_KEY}=${TEST_SCOPE_VALUE}" \
  "${PROD_TIER_KEY}=${DEV_TIER_VALUE}" \
  --overwrite
kubectl get ns "${PROD_NS}" "${DEV_NS}" --show-labels

banner "Baseline workload in prod (before policy)"
cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payments
  namespace: ${PROD_NS}
  labels:
    app: payments
spec:
  replicas: 1
  selector:
    matchLabels:
      app: payments
  template:
    metadata:
      labels:
        app: payments
    spec:
      containers:
        - name: app
          image: nginx:1.27
YAML
kubectl -n "${PROD_NS}" rollout status deploy/payments --timeout=120s

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: payments-sts
  namespace: ${PROD_NS}
  labels:
    app: payments-sts
spec:
  serviceName: payments-sts
  replicas: 1
  selector:
    matchLabels:
      app: payments-sts
  template:
    metadata:
      labels:
        app: payments-sts
    spec:
      containers:
        - name: app
          image: nginx:1.27
          ports:
            - containerPort: 80
YAML
kubectl -n "${PROD_NS}" rollout status sts/payments-sts --timeout=180s

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agents
  namespace: ${PROD_NS}
  labels:
    app: agents
spec:
  selector:
    matchLabels:
      app: agents
  template:
    metadata:
      labels:
        app: agents
    spec:
      containers:
        - name: agent
          image: nginx:1.27
YAML
kubectl -n "${PROD_NS}" rollout status ds/agents --timeout=180s

cat <<YAML | kubectl apply -f -
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly
  namespace: ${PROD_NS}
  labels:
    app: nightly
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: job
              image: nginx:1.27
YAML

banner "Apply deterministic MaintenanceWindow (outside now)"
cat <<YAML | kubectl apply -f -
apiVersion: freeze-operator.io/v1alpha1
kind: MaintenanceWindow
metadata:
  name: prod-deny-outside
  labels:
    ${TEST_LABEL_KEY}: "${TEST_LABEL_VALUE}"
    ${TEST_SCOPE_KEY}: "${TEST_SCOPE_VALUE}"
spec:
  timezone: UTC
  mode: DenyOutsideWindows
  windows:
    - name: jan-1-only
      schedule: "0 0 1 1 *"
      duration: 1h
  target:
    namespaceSelector:
      matchLabels:
        ${TEST_SCOPE_KEY}: ${TEST_SCOPE_VALUE}
        ${PROD_TIER_KEY}: ${PROD_TIER_VALUE}
    kinds: [Deployment, StatefulSet, DaemonSet, CronJob]
  rules:
    deny: [ROLL_OUT, SCALE, CREATE, DELETE]
  message:
    reason: "Production changes are allowed only during maintenance windows"
YAML

banner "PROD validation (expect DENY)"
fail=0
run_expect_deny() {
  local desc="$1"; shift
  echo "--- ${desc}"
  set +e
  local out
  if command -v timeout >/dev/null 2>&1; then
    out="$(timeout 25s "$@" 2>&1)"
  else
    out="$($@ 2>&1)"
  fi
  local rc=$?
  set -e
  echo "${out}" | sed -n '1,25p'
  if [ ${rc} -eq 0 ]; then
    echo "RESULT: FAIL (command succeeded, expected deny)"
    fail=1
  else
    echo "RESULT: PASS (denied as expected)"
  fi
  echo
}

run_expect_deny 'scale deployment' kubectl -n "${PROD_NS}" scale deploy/payments --replicas=3
run_expect_deny 'rollout restart' kubectl -n "${PROD_NS}" rollout restart deploy/payments
run_expect_deny 'delete deployment' kubectl -n "${PROD_NS}" delete deploy/payments --wait=false
run_expect_deny 'create new deployment' kubectl -n "${PROD_NS}" create deployment should-not-create --image=nginx:1.27 --replicas=1

run_expect_deny 'scale statefulset' kubectl -n "${PROD_NS}" scale sts/payments-sts --replicas=2
run_expect_deny 'rollout restart statefulset' kubectl -n "${PROD_NS}" rollout restart sts/payments-sts
run_expect_deny 'delete statefulset' kubectl -n "${PROD_NS}" delete sts/payments-sts --wait=false
run_expect_deny 'create new statefulset' bash -lc "cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: should-not-create-sts
  namespace: ${PROD_NS}
spec:
  serviceName: should-not-create-sts
  replicas: 1
  selector:
    matchLabels:
      app: should-not-create-sts
  template:
    metadata:
      labels:
        app: should-not-create-sts
    spec:
      containers:
        - name: app
          image: nginx:1.27
YAML"

run_expect_deny 'rollout restart daemonset' kubectl -n "${PROD_NS}" rollout restart ds/agents
run_expect_deny 'delete daemonset' kubectl -n "${PROD_NS}" delete ds/agents --wait=false
run_expect_deny 'create new daemonset' bash -lc "cat <<'YAML' | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: should-not-create-ds
  namespace: ${PROD_NS}
spec:
  selector:
    matchLabels:
      app: should-not-create-ds
  template:
    metadata:
      labels:
        app: should-not-create-ds
    spec:
      containers:
        - name: agent
          image: nginx:1.27
YAML"

run_expect_deny 'update cronjob schedule' kubectl -n "${PROD_NS}" patch cronjob nightly --type=merge -p '{"spec":{"schedule":"*/5 * * * *"}}'
run_expect_deny 'delete cronjob' kubectl -n "${PROD_NS}" delete cronjob nightly --wait=false
run_expect_deny 'create new cronjob' kubectl -n "${PROD_NS}" create cronjob should-not-create-cj --schedule='*/5 * * * *' --image=nginx:1.27

banner "DEV validation (expect ALLOW)"
cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payments
  namespace: ${DEV_NS}
  labels:
    app: payments
spec:
  replicas: 1
  selector:
    matchLabels:
      app: payments
  template:
    metadata:
      labels:
        app: payments
    spec:
      containers:
        - name: app
          image: nginx:1.27
YAML

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: payments-sts
  namespace: ${DEV_NS}
  labels:
    app: payments-sts
spec:
  serviceName: payments-sts
  replicas: 1
  selector:
    matchLabels:
      app: payments-sts
  template:
    metadata:
      labels:
        app: payments-sts
    spec:
      containers:
        - name: app
          image: nginx:1.27
          ports:
            - containerPort: 80
YAML

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agents
  namespace: ${DEV_NS}
  labels:
    app: agents
spec:
  selector:
    matchLabels:
      app: agents
  template:
    metadata:
      labels:
        app: agents
    spec:
      containers:
        - name: agent
          image: nginx:1.27
YAML

cat <<YAML | kubectl apply -f -
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly
  namespace: ${DEV_NS}
  labels:
    app: nightly
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: job
              image: nginx:1.27
YAML

run_expect_allow 'dev scale deployment' kubectl -n "${DEV_NS}" scale deploy/payments --replicas=2
run_expect_allow 'dev rollout restart deployment' kubectl -n "${DEV_NS}" rollout restart deploy/payments
run_expect_allow 'dev delete deployment' kubectl -n "${DEV_NS}" delete deploy/payments --wait=false

kubectl -n "${DEV_NS}" rollout status sts/payments-sts --timeout=180s
run_expect_allow 'dev scale statefulset' kubectl -n "${DEV_NS}" scale sts/payments-sts --replicas=2
run_expect_allow 'dev rollout restart statefulset' kubectl -n "${DEV_NS}" rollout restart sts/payments-sts
run_expect_allow 'dev delete statefulset' kubectl -n "${DEV_NS}" delete sts/payments-sts --wait=false

kubectl -n "${DEV_NS}" rollout status ds/agents --timeout=180s
run_expect_allow 'dev rollout restart daemonset' kubectl -n "${DEV_NS}" rollout restart ds/agents
run_expect_allow 'dev delete daemonset' kubectl -n "${DEV_NS}" delete ds/agents --wait=false

run_expect_allow 'dev update cronjob schedule' kubectl -n "${DEV_NS}" patch cronjob nightly --type=merge -p '{"spec":{"schedule":"*/10 * * * *"}}'
run_expect_allow 'dev delete cronjob' kubectl -n "${DEV_NS}" delete cronjob nightly --wait=false

echo
banner "Manager logs (tail)"
kubectl -n kube-freeze-operator-system logs deployment/kube-freeze-operator-controller-manager -c manager --tail=160 || true

echo
if [ "${fail}" -eq 0 ]; then
  echo "ALL CHECKS: PASS"
else
  echo "ALL CHECKS: FAIL"
  exit 2
fi
