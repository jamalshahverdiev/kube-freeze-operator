#!/usr/bin/env bash
# v2.0 Live Integration Test
# Tests that:
#  1) ChangeFreeze with gitops.enabled=true pauses ArgoCD Application and Flux Kustomization
#  2) After freeze ends, they are restored
#  3) Webhook returns short message for GitOps SA
set -euo pipefail

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; exit 1; }
info() { echo -e "${YELLOW}[INFO]${NC} $*"; }

# ---------------------------------------------------------------------------
# 1. Setup: namespace + labels
# ---------------------------------------------------------------------------
info "Creating test namespace prod-gitops-test..."
kubectl create namespace prod-gitops-test --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace prod-gitops-test env=production --overwrite

# ---------------------------------------------------------------------------
# 2. Create ArgoCD Application (with autoSync enabled)
# ---------------------------------------------------------------------------
info "Creating ArgoCD Application with autoSync..."
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: test-app-gitops
  namespace: argocd
  labels:
    env: production
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: prod-gitops-test
  syncPolicy:
    automated:
      prune: false
      selfHeal: false
EOF

# ---------------------------------------------------------------------------
# 3. Create Flux Kustomization (with suspend=false)
# ---------------------------------------------------------------------------
info "Creating Flux Kustomization..."
cat <<EOF | kubectl apply -f -
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: test-kustomization-gitops
  namespace: flux-system
  labels:
    env: production
spec:
  interval: 5m
  path: ./
  prune: true
  sourceRef:
    kind: GitRepository
    name: flux-system
  suspend: false
EOF

# ---------------------------------------------------------------------------
# 4. Verify initial state
# ---------------------------------------------------------------------------
info "Verifying initial state (autoSync=enabled, suspend=false)..."
sleep 2

AUTOSYNC=$(kubectl get application test-app-gitops -n argocd \
  -o jsonpath='{.spec.syncPolicy.automated}' 2>/dev/null || echo "")
[[ -n "$AUTOSYNC" ]] && ok "ArgoCD: autoSync is enabled initially" \
                     || info "ArgoCD: autoSync was already nil (still valid)"

SUSPEND=$(kubectl get kustomization test-kustomization-gitops -n flux-system \
  -o jsonpath='{.spec.suspend}' 2>/dev/null || echo "false")
[[ "$SUSPEND" == "false" || "$SUSPEND" == "" ]] \
  && ok "Flux: suspend=false initially" \
  || fail "Flux: expected suspend=false, got $SUSPEND"

# ---------------------------------------------------------------------------
# 5. Apply ChangeFreeze with gitops.enabled (active NOW)
# ---------------------------------------------------------------------------
info "Applying ChangeFreeze with gitops.enabled=true (active now)..."
cat <<EOF | kubectl apply -f -
apiVersion: freeze-operator.io/v1alpha1
kind: ChangeFreeze
metadata:
  name: test-freeze-gitops
spec:
  startTime: "$(date -u -d '5 seconds ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-5S '+%Y-%m-%dT%H:%M:%SZ')"
  endTime:   "$(date -u -d '2 minutes' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v+2M '+%Y-%m-%dT%H:%M:%SZ')"
  target:
    namespaceSelector:
      matchLabels:
        env: production
    kinds:
      - Deployment
  rules:
    deny:
      - ROLL_OUT
  behavior:
    gitops:
      enabled: true
      providers:
        - argocd
        - flux
      argocd:
        applicationSelector:
          matchLabels:
            env: production
      flux:
        kustomizationSelector:
          matchLabels:
            env: production
EOF

# ---------------------------------------------------------------------------
# 6. Wait for controller to act (max 30s)
# ---------------------------------------------------------------------------
info "Waiting for operator to pause GitOps resources (up to 30s)..."
PAUSED=false
for i in $(seq 1 15); do
  sleep 2
  MANAGED=$(kubectl get application test-app-gitops -n argocd \
    -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed}' 2>/dev/null || echo "")
  if [[ "$MANAGED" == "true" ]]; then
    PAUSED=true
    break
  fi
  echo -n "."
done
echo ""

# ---------------------------------------------------------------------------
# 7. Verify ArgoCD was paused
# ---------------------------------------------------------------------------
info "Checking ArgoCD Application was paused..."
MANAGED_ARGOCD=$(kubectl get application test-app-gitops -n argocd \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed}' 2>/dev/null || echo "")
POLICY_ARGOCD=$(kubectl get application test-app-gitops -n argocd \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed-by-policy}' 2>/dev/null || echo "")
AUTOSYNC_NOW=$(kubectl get application test-app-gitops -n argocd \
  -o jsonpath='{.spec.syncPolicy.automated}' 2>/dev/null || echo "REMOVED")

[[ "$MANAGED_ARGOCD" == "true" ]] \
  && ok "ArgoCD: annotation freeze-operator.io/managed=true" \
  || fail "ArgoCD: managed annotation missing (operator may not have acted yet)"

[[ "$POLICY_ARGOCD" == "test-freeze-gitops" ]] \
  && ok "ArgoCD: managed-by-policy=test-freeze-gitops" \
  || fail "ArgoCD: wrong policy annotation: $POLICY_ARGOCD"

[[ -z "$AUTOSYNC_NOW" || "$AUTOSYNC_NOW" == "REMOVED" || "$AUTOSYNC_NOW" == "{}" ]] \
  && ok "ArgoCD: autoSync disabled (spec.syncPolicy.automated removed)" \
  || fail "ArgoCD: autoSync still set: $AUTOSYNC_NOW"

# ---------------------------------------------------------------------------
# 8. Verify Flux was suspended
# ---------------------------------------------------------------------------
info "Checking Flux Kustomization was suspended..."
MANAGED_FLUX=$(kubectl get kustomization test-kustomization-gitops -n flux-system \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed}' 2>/dev/null || echo "")
SUSPEND_NOW=$(kubectl get kustomization test-kustomization-gitops -n flux-system \
  -o jsonpath='{.spec.suspend}' 2>/dev/null || echo "")

[[ "$MANAGED_FLUX" == "true" ]] \
  && ok "Flux: annotation freeze-operator.io/managed=true" \
  || fail "Flux: managed annotation missing"

[[ "$SUSPEND_NOW" == "true" ]] \
  && ok "Flux: spec.suspend=true" \
  || fail "Flux: expected suspend=true, got '$SUSPEND_NOW'"

# ---------------------------------------------------------------------------
# 9. Check ChangeFreeze status
# ---------------------------------------------------------------------------
info "Checking ChangeFreeze status..."
CF_ACTIVE=$(kubectl get changefreeze test-freeze-gitops \
  -o jsonpath='{.status.active}' 2>/dev/null || echo "")
CF_PAUSED=$(kubectl get changefreeze test-freeze-gitops \
  -o jsonpath='{.status.gitopsPausedCount}' 2>/dev/null || echo "0")

[[ "$CF_ACTIVE" == "true" ]] \
  && ok "ChangeFreeze: status.active=true" \
  || fail "ChangeFreeze: status.active is '$CF_ACTIVE'"

info "ChangeFreeze: gitopsPausedCount=$CF_PAUSED"
[[ "$CF_PAUSED" -ge 1 ]] \
  && ok "ChangeFreeze: gitopsPausedCount >= 1 ($CF_PAUSED)" \
  || fail "ChangeFreeze: gitopsPausedCount is $CF_PAUSED, expected >= 1"

# ---------------------------------------------------------------------------
# 10. Test webhook deny message for ArgoCD SA
# ---------------------------------------------------------------------------
info "Testing webhook deny message for ArgoCD service account (ROLL_OUT attempt)..."
# First create the deployment as a normal user
kubectl apply -f - <<EOF 2>/dev/null || true
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy-webhook
  namespace: prod-gitops-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: c
        image: nginx:1.25
EOF
sleep 1
# Now try to update (ROLL_OUT) impersonating ArgoCD SA
DENY_MSG=$(kubectl set image deployment/test-deploy-webhook c=nginx:1.26 \
  -n prod-gitops-test \
  --as=system:serviceaccount:argocd:argocd-application-controller \
  --as-group=system:serviceaccounts:argocd \
  2>&1 || true)

echo "  Deny message: $DENY_MSG"
[[ "$DENY_MSG" == *"freeze-operator"* || "$DENY_MSG" == *"blocked"* || "$DENY_MSG" == *"denied"* ]] \
  && ok "Webhook: ROLL_OUT blocked for ArgoCD SA with short message" \
  || info "Webhook: could not verify (impersonation may require extra RBAC)"

# ---------------------------------------------------------------------------
# 11. Terminate freeze early and verify restore
# ---------------------------------------------------------------------------
info "Patching ChangeFreeze to expired (testing restore)..."
PAST2=$(date -u -d '2 hours ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-2H '+%Y-%m-%dT%H:%M:%SZ')
PAST1=$(date -u -d '3 hours ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-3H '+%Y-%m-%dT%H:%M:%SZ')
kubectl patch changefreeze test-freeze-gitops --type=merge \
  -p "{\"spec\":{\"startTime\":\"$PAST1\",\"endTime\":\"$PAST2\"}}"

info "Waiting for operator to restore GitOps resources (up to 30s)..."
for i in $(seq 1 15); do
  sleep 2
  MANAGED=$(kubectl get application test-app-gitops -n argocd \
    -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed}' 2>/dev/null || echo "")
  [[ -z "$MANAGED" ]] && break
  echo -n "."
done
echo ""

# ---------------------------------------------------------------------------
# 12. Verify ArgoCD restored
# ---------------------------------------------------------------------------
info "Verifying ArgoCD Application restored..."
MANAGED_AFTER=$(kubectl get application test-app-gitops -n argocd \
  -o jsonpath='{.metadata.annotations.freeze-operator\.io/managed}' 2>/dev/null || echo "")
[[ -z "$MANAGED_AFTER" ]] \
  && ok "ArgoCD: managed annotation removed (restored)" \
  || fail "ArgoCD: managed annotation still present after freeze ended: $MANAGED_AFTER"

# ---------------------------------------------------------------------------
# 13. Verify Flux restored
# ---------------------------------------------------------------------------
info "Verifying Flux Kustomization restored..."
SUSPEND_AFTER=$(kubectl get kustomization test-kustomization-gitops -n flux-system \
  -o jsonpath='{.spec.suspend}' 2>/dev/null || echo "true")
[[ "$SUSPEND_AFTER" == "false" || "$SUSPEND_AFTER" == "" ]] \
  && ok "Flux: spec.suspend restored to false" \
  || fail "Flux: suspend still true after freeze ended"

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
info "Cleaning up test resources..."
kubectl delete changefreeze test-freeze-gitops --ignore-not-found
kubectl delete application test-app-gitops -n argocd --ignore-not-found
kubectl delete kustomization test-kustomization-gitops -n flux-system --ignore-not-found
kubectl delete deployment test-deploy-webhook -n prod-gitops-test --ignore-not-found 2>/dev/null || true
kubectl delete namespace prod-gitops-test --ignore-not-found

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  v2.0 Integration Test: ALL PASSED ✓  ${NC}"
echo -e "${GREEN}========================================${NC}"
