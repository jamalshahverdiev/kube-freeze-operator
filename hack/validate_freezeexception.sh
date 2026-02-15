#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

IMG_DEFAULT="jamalshahverdiev/kube-freeze-operator:v1.0.4"
IMG="${IMG:-$IMG_DEFAULT}"
REDEPLOY="${REDEPLOY:-true}"

PROD_NS="${PROD_NS:-prod-freezeexception-test}"

CURRENT_USER="${CURRENT_USER:-$(kubectl config view --minify -o jsonpath='{.contexts[0].context.user}' 2>/dev/null || true)}"
CURRENT_USER="${CURRENT_USER:-kubernetes-admin}"

CF_NAME="${CF_NAME:-validate-exception-changefreeze}"
EX_NAME="${EX_NAME:-validate-freezeexception}"

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
kubectl delete freezeexception "${EX_NAME}" --ignore-not-found
kubectl delete changefreeze "${CF_NAME}" --ignore-not-found
# Namespace
if kubectl get ns "${PROD_NS}" >/dev/null 2>&1; then
  kubectl delete ns "${PROD_NS}" --wait=false || true
  kubectl wait --for=delete "ns/${PROD_NS}" --timeout=120s >/dev/null 2>&1 || true
fi

banner "Create prod test namespace (${PROD_NS})"
kubectl create ns "${PROD_NS}"
kubectl label ns "${PROD_NS}" freeze-operator-test=true freeze-test-scope=freezeexception freeze-test-tier=deny --overwrite
kubectl get ns "${PROD_NS}" --show-labels

banner "Create baseline workloads (labels are important for exception constraints)"
# Two deployments: one breakglass, one normal
cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: breakglass
  namespace: ${PROD_NS}
  labels:
    app: breakglass
    breakglass: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: breakglass
  template:
    metadata:
      labels:
        app: breakglass
        breakglass: "true"
    spec:
      containers:
        - name: app
          image: nginx:1.27
YAML

cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: normal
  namespace: ${PROD_NS}
  labels:
    app: normal
spec:
  replicas: 1
  selector:
    matchLabels:
      app: normal
  template:
    metadata:
      labels:
        app: normal
    spec:
      containers:
        - name: app
          image: nginx:1.27
YAML

kubectl -n "${PROD_NS}" rollout status deploy/breakglass --timeout=120s
kubectl -n "${PROD_NS}" rollout status deploy/normal --timeout=120s

banner "Apply active ChangeFreeze denying SCALE (baseline deny we will override)"
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
        freeze-test-scope: freezeexception
        freeze-test-tier: deny
    kinds: [Deployment]
  rules:
    deny: [SCALE, ROLL_OUT, CREATE, DELETE]
  message:
    reason: "ChangeFreeze active: no production changes"
YAML

banner "Verify deny without exception (expect DENY)"
fail=0
run_expect_deny 'scale breakglass deployment (no exception yet)' kubectl -n "${PROD_NS}" scale deploy/breakglass --replicas=2 || fail=1
run_expect_deny 'scale normal deployment (no exception yet)' kubectl -n "${PROD_NS}" scale deploy/normal --replicas=2 || fail=1

banner "Apply FreezeException allowing only SCALE for breakglass-labeled objects"
active_from="$(date -u -d '5 minutes ago' +%Y-%m-%dT%H:%M:%SZ)"
active_to="$(date -u -d '55 minutes' +%Y-%m-%dT%H:%M:%SZ)"
cat <<YAML | kubectl apply -f -
apiVersion: freeze-operator.io/v1alpha1
kind: FreezeException
metadata:
  name: ${EX_NAME}
spec:
  activeFrom: "${active_from}"
  activeTo: "${active_to}"
  target:
    namespaceSelector:
      matchLabels:
        freeze-test-scope: freezeexception
        freeze-test-tier: deny
    objectSelector:
      matchLabels:
        breakglass: "true"
    kinds: [Deployment]
  allow: [SCALE]
  constraints:
    requireLabels:
      breakglass: "true"
    allowedUsers:
      - ${CURRENT_USER}
  reason: "Breakglass approved for scaling"
  ticketURL: "https://example.invalid/ticket/123"
  approvedBy: "validate-script"
YAML

banner "Verify exception behavior"
# Breakglass deployment should now be allowed for SCALE
run_expect_allow 'scale breakglass deployment (expect ALLOW)' kubectl -n "${PROD_NS}" scale deploy/breakglass --replicas=2 || fail=1
# Normal deployment should still be denied
run_expect_deny 'scale normal deployment (expect DENY)' kubectl -n "${PROD_NS}" scale deploy/normal --replicas=2 || fail=1
# Non-allowed action should still be denied even for breakglass
run_expect_deny 'rollout restart breakglass (SCALE-only exception, expect DENY)' kubectl -n "${PROD_NS}" rollout restart deploy/breakglass || fail=1

banner "Cleanup policies"
kubectl delete freezeexception "${EX_NAME}" --ignore-not-found
kubectl delete changefreeze "${CF_NAME}" --ignore-not-found

banner "Manager logs (tail)"
kubectl -n kube-freeze-operator-system logs deployment/kube-freeze-operator-controller-manager -c manager --tail=220 || true

if [ "${fail}" -eq 0 ]; then
  echo "ALL CHECKS: PASS"
else
  echo "ALL CHECKS: FAIL"
  exit 2
fi
