#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

IMG_DEFAULT="jamalshahverdiev/kube-freeze-operator:v1.0.3"
IMG="${IMG:-$IMG_DEFAULT}"
REDEPLOY="${REDEPLOY:-true}"

PROD_NS="${PROD_NS:-prod-changefreeze-test}"
DEV_NS="${DEV_NS:-dev-changefreeze-test}"

CF_NAME="${CF_NAME:-validate-changefreeze}"

banner() {
  printf "\n== %s ==\n" "$1"
}

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
    return 1
  fi
  echo "RESULT: PASS (denied as expected)"
  echo
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
    return 1
  fi
  echo "RESULT: PASS (allowed as expected)"
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

banner "Cleanup previous test artifacts"
# Policies are cluster-scoped
kubectl delete changefreeze "${CF_NAME}" --ignore-not-found
# Namespaces are namespaced
for ns in "${PROD_NS}" "${DEV_NS}"; do
  if kubectl get ns "${ns}" >/dev/null 2>&1; then
    kubectl delete ns "${ns}" --wait=false || true
    kubectl wait --for=delete "ns/${ns}" --timeout=120s >/dev/null 2>&1 || true
  fi
done

banner "Create test namespaces (${PROD_NS}, ${DEV_NS})"
kubectl create ns "${PROD_NS}"
kubectl create ns "${DEV_NS}"
kubectl label ns "${PROD_NS}" freeze-operator-test=true freeze-test-scope=changefreeze freeze-test-tier=deny --overwrite
kubectl label ns "${DEV_NS}" freeze-operator-test=true freeze-test-scope=changefreeze freeze-test-tier=allow --overwrite
kubectl get ns "${PROD_NS}" "${DEV_NS}" --show-labels

banner "Baseline workloads (before ChangeFreeze)"
# PROD baseline
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

# DEV baseline (should remain unaffected)
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
kubectl -n "${DEV_NS}" rollout status deploy/payments --timeout=120s

banner "Apply active ChangeFreeze (deny in range)"
start_time="$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)"
end_time="$(date -u -d '1 hour' +%Y-%m-%dT%H:%M:%SZ)"
cat <<YAML | kubectl apply -f -
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: ${CF_NAME}
spec:
  startTime: "${start_time}"
  endTime: "${end_time}"
  target:
    namespaceSelector:
      matchLabels:
        freeze-test-scope: changefreeze
        freeze-test-tier: deny
    kinds: [Deployment, StatefulSet, DaemonSet, CronJob]
  rules:
    deny: [ROLL_OUT, SCALE, CREATE, DELETE]
  message:
    reason: "ChangeFreeze active: no production changes"
YAML

banner "PROD validation (expect DENY)"
fail=0

run_expect_deny 'deployment scale' kubectl -n "${PROD_NS}" scale deploy/payments --replicas=2 || fail=1
run_expect_deny 'deployment rollout restart' kubectl -n "${PROD_NS}" rollout restart deploy/payments || fail=1
run_expect_deny 'deployment delete' kubectl -n "${PROD_NS}" delete deploy/payments --wait=false || fail=1
run_expect_deny 'deployment create' kubectl -n "${PROD_NS}" create deployment should-not-create --image=nginx:1.27 --replicas=1 || fail=1

run_expect_deny 'statefulset scale' kubectl -n "${PROD_NS}" scale sts/payments-sts --replicas=2 || fail=1
run_expect_deny 'statefulset rollout restart' kubectl -n "${PROD_NS}" rollout restart sts/payments-sts || fail=1
run_expect_deny 'statefulset delete' kubectl -n "${PROD_NS}" delete sts/payments-sts --wait=false || fail=1

run_expect_deny 'daemonset rollout restart' kubectl -n "${PROD_NS}" rollout restart ds/agents || fail=1
run_expect_deny 'daemonset delete' kubectl -n "${PROD_NS}" delete ds/agents --wait=false || fail=1

run_expect_deny 'cronjob patch schedule' kubectl -n "${PROD_NS}" patch cronjob nightly --type=merge -p '{"spec":{"schedule":"*/5 * * * *"}}' || fail=1
run_expect_deny 'cronjob delete' kubectl -n "${PROD_NS}" delete cronjob nightly --wait=false || fail=1
run_expect_deny 'cronjob create' kubectl -n "${PROD_NS}" create cronjob should-not-create-cj --schedule='*/5 * * * *' --image=nginx:1.27 || fail=1

banner "DEV validation (expect ALLOW)"
run_expect_allow 'dev deployment scale' kubectl -n "${DEV_NS}" scale deploy/payments --replicas=2 || fail=1
run_expect_allow 'dev deployment rollout restart' kubectl -n "${DEV_NS}" rollout restart deploy/payments || fail=1

banner "Remove ChangeFreeze and re-validate allows (expect ALLOW)"
kubectl delete changefreeze "${CF_NAME}" --wait=true
run_expect_allow 'prod deployment scale after delete policy' kubectl -n "${PROD_NS}" scale deploy/payments --replicas=2 || fail=1

banner "Manager logs (tail)"
kubectl -n kube-freeze-operator-system logs deployment/kube-freeze-operator-controller-manager -c manager --tail=180 || true

if [ "${fail}" -eq 0 ]; then
  echo "ALL CHECKS: PASS"
else
  echo "ALL CHECKS: FAIL"
  exit 2
fi
