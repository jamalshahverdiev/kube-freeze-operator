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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
)

func newTestServer(t *testing.T, objs ...runtime.Object) *Server {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := freezev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	runtimeObjs := make([]runtime.Object, len(objs))
	copy(runtimeObjs, objs)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(runtimeObjs...).Build()
	return NewServer(cl, ":0")
}

func postEvaluate(srv *Server, body EvaluateRequest) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvaluate(w, req)
	return w
}

func getEvaluate(srv *Server, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/evaluate?"+query, nil)
	w := httptest.NewRecorder()
	srv.handleEvaluateGET(w, req)
	return w
}

func TestEvaluate_AllowWhenNoFreeze(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	w := postEvaluate(srv, EvaluateRequest{
		Namespace: "default",
		Kind:      "Deployment",
		Action:    "CREATE",
	})
	g.Expect(w.Code).To(Equal(http.StatusOK))

	var resp EvaluateResponse
	g.Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	g.Expect(resp.Allow).To(BeTrue())
	g.Expect(resp.Reason).To(BeEmpty())
	g.Expect(resp.EvaluatedAt).NotTo(BeEmpty())
}

func TestEvaluate_DenyDuringFreeze(t *testing.T) {
	g := NewWithT(t)

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "prod"}}}

	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "release-freeze"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.NewTime(start),
			EndTime:   metav1.NewTime(end),
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
				Kinds: []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{
				Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate, freezev1alpha1.ActionRollout},
			},
		},
	}

	srv := newTestServer(t, ns, cf)

	w := postEvaluate(srv, EvaluateRequest{
		Namespace: "prod",
		Kind:      "Deployment",
		Action:    "CREATE",
	})
	g.Expect(w.Code).To(Equal(http.StatusOK))

	var resp EvaluateResponse
	g.Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	g.Expect(resp.Allow).To(BeFalse())
	g.Expect(resp.MatchedPolicy).To(Equal("release-freeze"))
	g.Expect(resp.PolicyKind).To(Equal("ChangeFreeze"))
	g.Expect(resp.FreezeEndTime).NotTo(BeNil())
}

func TestEvaluate_BadRequest_MissingFields(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	tests := []struct {
		name string
		req  EvaluateRequest
	}{
		{"missing namespace", EvaluateRequest{Kind: "Deployment", Action: "CREATE"}},
		{"missing kind", EvaluateRequest{Namespace: "default", Action: "CREATE"}},
		{"missing action", EvaluateRequest{Namespace: "default", Kind: "Deployment"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postEvaluate(srv, tt.req)
			g.Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	}
}

func TestEvaluate_BadRequest_InvalidKind(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	w := postEvaluate(srv, EvaluateRequest{
		Namespace: "default",
		Kind:      "Pod",
		Action:    "CREATE",
	})
	g.Expect(w.Code).To(Equal(http.StatusBadRequest))
}

func TestEvaluate_BadRequest_InvalidAction(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	w := postEvaluate(srv, EvaluateRequest{
		Namespace: "default",
		Kind:      "Deployment",
		Action:    "RESTART",
	})
	g.Expect(w.Code).To(Equal(http.StatusBadRequest))
}

func TestEvaluate_BadRequest_InvalidJSON(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleEvaluate(w, req)
	g.Expect(w.Code).To(Equal(http.StatusBadRequest))
}

func TestEvaluate_GET(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	w := getEvaluate(srv, "namespace=default&kind=Deployment&action=CREATE")
	g.Expect(w.Code).To(Equal(http.StatusOK))

	var resp EvaluateResponse
	g.Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	g.Expect(resp.Allow).To(BeTrue())
}

func TestEvaluate_GET_MissingFields(t *testing.T) {
	g := NewWithT(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	srv := newTestServer(t, ns)

	w := getEvaluate(srv, "namespace=default&kind=Deployment")
	g.Expect(w.Code).To(Equal(http.StatusBadRequest))
}

func TestHealthz(t *testing.T) {
	g := NewWithT(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	g.Expect(w.Code).To(Equal(http.StatusOK))
	g.Expect(w.Body.String()).To(Equal("ok"))
}

func TestEvaluate_AllowWithException(t *testing.T) {
	g := NewWithT(t)

	now := time.Now().UTC()
	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "prod"}}}

	cf := &freezev1alpha1.ChangeFreeze{
		ObjectMeta: metav1.ObjectMeta{Name: "release-freeze"},
		Spec: freezev1alpha1.ChangeFreezeSpec{
			StartTime: metav1.NewTime(start),
			EndTime:   metav1.NewTime(end),
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
				Kinds: []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Rules: freezev1alpha1.PolicyRulesSpec{
				Deny: []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			},
		},
	}

	fe := &freezev1alpha1.FreezeException{
		ObjectMeta: metav1.ObjectMeta{Name: "hotfix-exception"},
		Spec: freezev1alpha1.FreezeExceptionSpec{
			ActiveFrom: metav1.NewTime(start),
			ActiveTo:   metav1.NewTime(end),
			Target: freezev1alpha1.TargetSpec{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"env": "prod"},
				},
				Kinds: []freezev1alpha1.TargetKind{freezev1alpha1.TargetKindDeployment},
			},
			Allow:  []freezev1alpha1.Action{freezev1alpha1.ActionCreate},
			Reason: "Hotfix for critical bug",
		},
	}

	srv := newTestServer(t, ns, cf, fe)

	w := postEvaluate(srv, EvaluateRequest{
		Namespace: "prod",
		Kind:      "Deployment",
		Action:    "CREATE",
	})
	g.Expect(w.Code).To(Equal(http.StatusOK))

	var resp EvaluateResponse
	g.Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	g.Expect(resp.Allow).To(BeTrue())
}
