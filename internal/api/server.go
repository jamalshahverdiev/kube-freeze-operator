/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
	freezemetrics "github.com/jamalshahverdiev/kube-freeze-operator/internal/metrics"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/policy"
)

var log = ctrl.Log.WithName("api")

// EvaluateRequest is the JSON body for POST /v1/evaluate.
type EvaluateRequest struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Action    string `json:"action"`
	Name      string `json:"name,omitempty"`
}

// EvaluateResponse is the JSON reply from POST /v1/evaluate.
type EvaluateResponse struct {
	Allow         bool    `json:"allow"`
	Reason        string  `json:"reason,omitempty"`
	MatchedPolicy string  `json:"matchedPolicy,omitempty"`
	PolicyKind    string  `json:"policyKind,omitempty"`
	FreezeEndTime *string `json:"freezeEndTime,omitempty"`
	NextAllowed   *string `json:"nextAllowedTime,omitempty"`
	EvaluatedAt   string  `json:"evaluatedAt"`
}

// Server serves the freeze-operator CI helper API.
type Server struct {
	client   client.Reader
	addr     string
	authMode AuthMode
	auth     *TokenAuthMiddleware
}

// NewServer creates a new API server.
func NewServer(c client.Reader, addr string, opts ...ServerOption) *Server {
	s := &Server{client: c, addr: addr, authMode: AuthModeNone}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithTokenAuth enables TokenReview authentication.
func WithTokenAuth(auth *TokenAuthMiddleware) ServerOption {
	return func(s *Server) {
		s.authMode = AuthModeToken
		s.auth = auth
	}
}

// Start runs the HTTP server. It blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/evaluate", s.handleEvaluate)
	mux.HandleFunc("GET /v1/evaluate", s.handleEvaluateGET)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	var handler http.Handler = mux
	if s.authMode == AuthModeToken && s.auth != nil {
		handler = s.auth.Wrap(mux)
		log.Info("API server authentication enabled", "mode", s.authMode)
	}

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("starting API server", "addr", s.addr)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleEvaluateGET(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := EvaluateRequest{
		Namespace: q.Get("namespace"),
		Kind:      q.Get("kind"),
		Action:    q.Get("action"),
		Name:      q.Get("name"),
	}
	s.evaluate(w, r, req)
}

func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	s.evaluate(w, r, req)
}

func (s *Server) evaluate(w http.ResponseWriter, r *http.Request, req EvaluateRequest) {
	if req.Namespace == "" || req.Kind == "" || req.Action == "" {
		freezemetrics.APIErrors.WithLabelValues("bad_request").Inc()
		writeError(w, http.StatusBadRequest, "namespace, kind, and action are required")
		return
	}

	kind, ok := parseKind(req.Kind)
	if !ok {
		freezemetrics.APIErrors.WithLabelValues("bad_request").Inc()
		writeError(w, http.StatusBadRequest, "unsupported kind: "+req.Kind+"; valid: Deployment, StatefulSet, DaemonSet, CronJob")
		return
	}

	action, ok := parseAction(req.Action)
	if !ok {
		freezemetrics.APIErrors.WithLabelValues("bad_request").Inc()
		writeError(w, http.StatusBadRequest, "unsupported action: "+req.Action+"; valid: CREATE, DELETE, ROLL_OUT, SCALE")
		return
	}

	now := time.Now().UTC()
	in := policy.Input{
		Now:       now,
		Namespace: req.Namespace,
		Kind:      kind,
		Action:    action,
	}

	eval := &policy.Evaluator{Client: s.client}
	dec, err := eval.Evaluate(r.Context(), in)
	if err != nil {
		freezemetrics.APIErrors.WithLabelValues("internal").Inc()
		log.Error(err, "policy evaluation failed", "namespace", req.Namespace, "kind", req.Kind, "action", req.Action)
		writeError(w, http.StatusInternalServerError, "evaluation error: "+err.Error())
		return
	}

	elapsed := time.Since(now).Seconds()
	freezemetrics.APILatency.Observe(elapsed)

	decision := "allow"
	if !dec.Allowed {
		decision = "deny"
	}
	freezemetrics.APIRequests.WithLabelValues(decision, req.Namespace, req.Kind, req.Action).Inc()

	resp := EvaluateResponse{
		Allow:       dec.Allowed,
		EvaluatedAt: now.Format(time.RFC3339),
	}

	if !dec.Allowed {
		resp.Reason = dec.Reason
		if dec.MatchedPolicy != nil {
			resp.MatchedPolicy = dec.MatchedPolicy.Name
			resp.PolicyKind = string(dec.MatchedPolicy.Kind)
		}
		if dec.FreezeEndTime != nil {
			t := dec.FreezeEndTime.Format(time.RFC3339)
			resp.FreezeEndTime = &t
		}
		if dec.NextAllowedTime != nil {
			t := dec.NextAllowedTime.Format(time.RFC3339)
			resp.NextAllowed = &t
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func parseKind(s string) (freezev1alpha1.TargetKind, bool) {
	switch freezev1alpha1.TargetKind(s) {
	case freezev1alpha1.TargetKindDeployment:
		return freezev1alpha1.TargetKindDeployment, true
	case freezev1alpha1.TargetKindStatefulSet:
		return freezev1alpha1.TargetKindStatefulSet, true
	case freezev1alpha1.TargetKindDaemonSet:
		return freezev1alpha1.TargetKindDaemonSet, true
	case freezev1alpha1.TargetKindCronJob:
		return freezev1alpha1.TargetKindCronJob, true
	}
	return "", false
}

func parseAction(s string) (freezev1alpha1.Action, bool) {
	switch freezev1alpha1.Action(s) {
	case freezev1alpha1.ActionCreate:
		return freezev1alpha1.ActionCreate, true
	case freezev1alpha1.ActionDelete:
		return freezev1alpha1.ActionDelete, true
	case freezev1alpha1.ActionRollout:
		return freezev1alpha1.ActionRollout, true
	case freezev1alpha1.ActionScale:
		return freezev1alpha1.ActionScale, true
	}
	return "", false
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
