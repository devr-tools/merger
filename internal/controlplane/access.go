package controlplane

import (
	"errors"
	"net/http"
	"strings"

	"github.com/devr-tools/merger/internal/access"
)

const bearerChallenge = "Bearer"

// AccessMiddleware authenticates control-plane requests and enforces the role
// required by each HTTP operation. Health checks remain public.
func AccessMiddleware(authenticator access.Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		principal, err := authenticator.Authenticate(r.Header.Get("Authorization"))
		if err != nil {
			w.Header().Set("WWW-Authenticate", bearerChallenge)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		role := access.RoleReader
		if isEvidenceUpdate(r) {
			role = access.RoleEvidenceWriter
		}
		if err := access.Authorize(principal, role); err != nil {
			if errors.Is(err, access.ErrUnauthenticated) {
				w.Header().Set("WWW-Authenticate", bearerChallenge)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r.WithContext(access.WithPrincipal(r.Context(), principal)))
	})
}

func isEvidenceUpdate(r *http.Request) bool {
	if r.Method != http.MethodPut {
		return false
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/change-packets/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 3 && parts[0] != "" && parts[1] == "evidence" && parts[2] != ""
}
