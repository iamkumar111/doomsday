package dashboard

import (
	"net/http"
	"strings"
)

func (s *Server) requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.APIToken == "" {
			next(w, r)
			return
		}
		if tokenFromRequest(r) != s.APIToken {
			w.Header().Set("WWW-Authenticate", `Bearer realm="sh-mvdos-lab"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func tokenFromRequest(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-Lab-Token")); t != "" {
		return t
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}