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
	"net/http"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// AuthMode defines how the API server authenticates requests.
type AuthMode string

const (
	AuthModeNone  AuthMode = "none"
	AuthModeToken AuthMode = "token"
)

// TokenAuthMiddleware validates Bearer tokens via Kubernetes TokenReview.
type TokenAuthMiddleware struct {
	clientset kubernetes.Interface
}

// NewTokenAuthMiddleware creates a middleware that validates SA tokens.
func NewTokenAuthMiddleware(cs kubernetes.Interface) *TokenAuthMiddleware {
	return &TokenAuthMiddleware{clientset: cs}
}

// Wrap returns an http.Handler that checks Bearer token before delegating.
func (m *TokenAuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}

		review := &authv1.TokenReview{
			Spec: authv1.TokenReviewSpec{
				Token: token,
			},
		}

		result, err := m.clientset.AuthenticationV1().TokenReviews().Create(
			r.Context(), review, metav1.CreateOptions{},
		)
		if err != nil {
			log.Error(err, "TokenReview failed")
			writeError(w, http.StatusInternalServerError, "authentication error")
			return
		}

		if !result.Status.Authenticated {
			writeError(w, http.StatusUnauthorized, "token not authenticated")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
