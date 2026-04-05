#!/usr/bin/env bash
# v3.0.1 Live Integration Test — TokenReview Authentication
# Requires: --api-auth-mode=token enabled on the controller-manager
# Tests:
#  1) Healthz without token → 200
#  2) POST /v1/evaluate without token → 401
#  3) GET /v1/evaluate without token → 401
#  4) POST with invalid token → 401
#  5) POST with valid SA token (allow) → 200
#  6) POST with valid SA token (deny) → 200
#  7) GET with valid SA token → 200
#  8) Cleanup
set -euo pipefail

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
PASS=0; FAIL=0
ok()   { echo -e "${GREEN}[PASS]${NC} $*"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}[FAIL]${NC} $*"; FAIL=$((FAIL+1)); }
info() { echo -e "${YELLOW}[INFO]${NC} $*"; }

NS="kube-freeze-operator-system"
API_PORT=8082
TEST_NS="v3-auth-test"
SA_NAME="freeze-auth-test-sa"
FREEZE_NAME="auth-integration-freeze"

# ---------------------------------------------------------------------------
# 0. Port-forward to API
# ---------------------------------------------------------------------------
info "Setting up port-forward on :${API_PORT}..."
POD=$(kubectl get pods -n "$NS" -l control-plane=controller-manager \
  --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
kubectl port-forward -n "$NS" "pod/$POD" ${API_PORT}:${API_PORT} &>/dev/null &
PF_PID=$!
sleep 3

cleanup() {
  kill "$PF_PID" 2>/dev/null || true
  kubectl delete changefreeze "$FREEZE_NAME" --ignore-not-found 2>/dev/null
  kubectl delete sa "$SA_NAME" -n default --ignore-not-found 2>/dev/null
  kubectl delete ns "$TEST_NS" --ignore-not-found --wait=false 2>/dev/null
}
trap cleanup EXIT

API="http://localhost:${API_PORT}"

# Verify auth mode is token
AUTH_ARGS=$(kubectl get deployment -n "$NS" kube-freeze-operator-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].args}')
if echo "$AUTH_ARGS" | grep -q 'api-auth-mode=token'; then
  info "Confirmed: --api-auth-mode=token is set"
else
  echo -e "${RED}ERROR: --api-auth-mode=token not found in deployment args. Aborting.${NC}"
  echo "Current args: $AUTH_ARGS"
  exit 1
fi

# ---------------------------------------------------------------------------
# 1. Healthz without token → 200
# ---------------------------------------------------------------------------
info "Test 1: GET /healthz without token"
RESP=$(curl -sf "${API}/healthz" 2>&1) || RESP=""
if [[ "$RESP" == "ok" ]]; then ok "healthz returns 200 without token"; else fail "healthz: got '$RESP'"; fi

# ---------------------------------------------------------------------------
# 2. POST without token → 401
# ---------------------------------------------------------------------------
info "Test 2: POST /v1/evaluate without token"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${API}/v1/evaluate" \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"default","kind":"Deployment","action":"CREATE"}')
if [[ "$HTTP_CODE" == "401" ]]; then ok "POST without token → 401"; else fail "expected 401, got $HTTP_CODE"; fi

# ---------------------------------------------------------------------------
# 3. GET without token → 401
# ---------------------------------------------------------------------------
info "Test 3: GET /v1/evaluate without token"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' \
  "${API}/v1/evaluate?namespace=default&kind=Deployment&action=CREATE")
if [[ "$HTTP_CODE" == "401" ]]; then ok "GET without token → 401"; else fail "expected 401, got $HTTP_CODE"; fi

# ---------------------------------------------------------------------------
# 4. POST with invalid token → 401
# ---------------------------------------------------------------------------
info "Test 4: POST with invalid Bearer token"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${API}/v1/evaluate" \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer totally-fake-token-12345' \
  -d '{"namespace":"default","kind":"Deployment","action":"CREATE"}')
if [[ "$HTTP_CODE" == "401" ]]; then ok "invalid token → 401"; else fail "expected 401, got $HTTP_CODE"; fi

# ---------------------------------------------------------------------------
# 5. Create SA + get valid token
# ---------------------------------------------------------------------------
info "Creating ServiceAccount ${SA_NAME}..."
kubectl create serviceaccount "$SA_NAME" -n default --dry-run=client -o yaml | kubectl apply -f -
TOKEN=$(kubectl create token "$SA_NAME" -n default --duration=10m)
TOKEN_LEN=${#TOKEN}
info "Got SA token (length=${TOKEN_LEN})"

# ---------------------------------------------------------------------------
# 6. POST with valid token — allow (no freeze active)
# ---------------------------------------------------------------------------
info "Test 5: POST with valid token (allow, no freeze)"
RESP=$(curl -sf "${API}/v1/evaluate" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"namespace":"default","kind":"Deployment","action":"CREATE"}' 2>&1) || RESP=""
ALLOW=$(echo "$RESP" | jq -r '.allow')
if [[ "$ALLOW" == "true" ]]; then ok "valid token → allow=true"; else fail "expected allow, got: $RESP"; fi

# ---------------------------------------------------------------------------
# 7. Create ChangeFreeze + test deny with token
# ---------------------------------------------------------------------------
info "Creating test namespace and ChangeFreeze..."
kubectl create namespace "$TEST_NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "$TEST_NS" env=authtest --overwrite

START=$(date -u -d '-5 minutes' '+%Y-%m-%dT%H:%M:%SZ')
END=$(date -u -d '+10 minutes' '+%Y-%m-%dT%H:%M:%SZ')

cat <<EOF | kubectl apply -f -
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: ${FREEZE_NAME}
spec:
  startTime: "$START"
  endTime: "$END"
  target:
    namespaceSelector:
      matchLabels:
        env: authtest
    kinds:
      - Deployment
      - StatefulSet
  rules:
    deny:
      - CREATE
      - ROLL_OUT
  message:
    reason: "auth integration test"
EOF
sleep 2

info "Test 6: POST with valid token (deny during freeze)"
RESP=$(curl -sf "${API}/v1/evaluate" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"namespace\":\"${TEST_NS}\",\"kind\":\"Deployment\",\"action\":\"CREATE\"}" 2>&1) || RESP=""
ALLOW=$(echo "$RESP" | jq -r '.allow')
POLICY=$(echo "$RESP" | jq -r '.matchedPolicy')
ENDTIME=$(echo "$RESP" | jq -r '.freezeEndTime // empty')
if [[ "$ALLOW" == "false" ]]; then ok "valid token → deny during freeze"; else fail "expected deny, got: $RESP"; fi
if [[ "$POLICY" == "${FREEZE_NAME}" ]]; then ok "matchedPolicy=${FREEZE_NAME}"; else fail "matchedPolicy: $POLICY"; fi
if [[ -n "$ENDTIME" ]]; then ok "freezeEndTime present"; else fail "freezeEndTime missing"; fi

# ---------------------------------------------------------------------------
# 8. GET with valid token → deny
# ---------------------------------------------------------------------------
info "Test 7: GET with valid token (deny)"
RESP=$(curl -sf "${API}/v1/evaluate?namespace=${TEST_NS}&kind=Deployment&action=CREATE" \
  -H "Authorization: Bearer $TOKEN" 2>&1) || RESP=""
ALLOW=$(echo "$RESP" | jq -r '.allow')
if [[ "$ALLOW" == "false" ]]; then ok "GET with token → deny"; else fail "GET expected deny, got: $RESP"; fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "============================================"
echo -e "Results: ${GREEN}${PASS} passed${NC}, ${RED}${FAIL} failed${NC}"
echo "============================================"

if [[ $FAIL -gt 0 ]]; then exit 1; fi
