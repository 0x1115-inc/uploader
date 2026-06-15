/**
uploader - A simple file upload server with S3-compatible storage support
Copyright (C) 2026 0x1115 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
**/

package httpapi

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is a private type for context keys in this package, preventing
// collisions with keys from other packages.
type contextKey int

const authEmailKey contextKey = iota

// requireAuth is a middleware that reads the X-Auth-Email header injected by
// oauth2-proxy after successful authentication. Requests that carry no valid
// email are rejected with HTTP 401 before the handler is invoked.
//
// Security considerations:
//
//   - This middleware MUST only be applied to routes that sit exclusively
//     behind the oauth2-proxy reverse proxy. If the service is reachable
//     directly (e.g. on an internal port) anyone could forge X-Auth-Email.
//     Enforce this with a network policy or firewall rule.
//
//   - oauth2-proxy strips X-Auth-Email from incoming client requests before
//     validating the session and re-injecting it upstream. Verify that your
//     oauth2-proxy configuration has --pass-user-headers=true and that the
//     upstream address is not reachable without going through the proxy.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.Header.Get("X-Auth-Email"))
		if !isValidAuthEmail(email) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), authEmailKey, email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authEmailFromContext retrieves the authenticated user's email from the request
// context. Returns an empty string outside of requireAuth-protected routes.
func authEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(authEmailKey).(string)
	return email
}

// isValidAuthEmail performs a minimal sanity check on the header value injected
// by oauth2-proxy. Full RFC 5321 validation is intentionally omitted — the
// identity provider has already validated the address. This check guards only
// against empty values and log-injection characters (\r, \n, \t).
func isValidAuthEmail(email string) bool {
	return email != "" &&
		strings.Contains(email, "@") &&
		!strings.ContainsAny(email, "\r\n\t")
}
