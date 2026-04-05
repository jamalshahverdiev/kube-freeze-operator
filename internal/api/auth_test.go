package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestTokenAuth_NoToken(t *testing.T) {
	g := NewWithT(t)
	cs := kubefake.NewClientset()
	mw := NewTokenAuthMiddleware(cs)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/evaluate?namespace=default&kind=Deployment&action=CREATE", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	g.Expect(w.Code).To(Equal(http.StatusUnauthorized))
	var resp map[string]string
	g.Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	g.Expect(resp["error"]).To(ContainSubstring("missing Bearer token"))
}

func TestTokenAuth_InvalidToken(t *testing.T) {
	g := NewWithT(t)
	cs := kubefake.NewClientset()
	cs.PrependReactor("create", "tokenreviews", func(_ kubetesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{Authenticated: false},
		}, nil
	})
	mw := NewTokenAuthMiddleware(cs)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/evaluate?namespace=default&kind=Deployment&action=CREATE", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	g.Expect(w.Code).To(Equal(http.StatusUnauthorized))
}

func TestTokenAuth_ValidToken(t *testing.T) {
	g := NewWithT(t)
	cs := kubefake.NewClientset()
	cs.PrependReactor("create", "tokenreviews", func(_ kubetesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				Authenticated: true,
				User: authv1.UserInfo{
					Username: "system:serviceaccount:ci:deployer",
				},
			},
		}, nil
	})
	mw := NewTokenAuthMiddleware(cs)

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/evaluate?namespace=default&kind=Deployment&action=CREATE", nil)
	req.Header.Set("Authorization", "Bearer valid-sa-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	g.Expect(w.Code).To(Equal(http.StatusOK))
	g.Expect(called).To(BeTrue())
}

func TestTokenAuth_HealthzBypass(t *testing.T) {
	g := NewWithT(t)
	cs := kubefake.NewClientset()
	mw := NewTokenAuthMiddleware(cs)

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Wrap(inner)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	g.Expect(w.Code).To(Equal(http.StatusOK))
	g.Expect(called).To(BeTrue())
}

func TestExtractBearerToken(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty", "", ""},
		{"no bearer", "Basic abc", ""},
		{"bearer", "Bearer mytoken123", "mytoken123"},
		{"bearer lowercase", "bearer mytoken", "mytoken"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			g.Expect(extractBearerToken(req)).To(Equal(tt.want))
		})
	}
}
