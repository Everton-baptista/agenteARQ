// Package dashboard serves a read-only, loopback-only view of the shared application.
package dashboard

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/application"
)

//go:embed all:assets
var assets embed.FS

// Handler requires a loopback Host and a local session even for static resources.
// The only API is GET status; viewing this page never launches a check or agent.
func Handler(root, host, token string) http.Handler {
	sub, _ := fs.Sub(assets, "assets")
	static := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != host {
			http.Error(w, "invalid local host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		if fetch := r.Header.Get("Sec-Fetch-Site"); fetch == "cross-site" {
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "read-only dashboard", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" && r.URL.Query().Get("token") != "" {
			if !sameToken(r.URL.Query().Get("token"), token) {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}
			// Not Secure on purpose: the listener is plain HTTP bound to 127.0.0.1, and a browser that
			// does not treat loopback HTTP as a secure context would drop the cookie and lock the
			// user out. HttpOnly, SameSite=Strict and the Host/Origin checks above carry the session.
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName(host), Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		cookie, err := r.Cookie(sessionCookieName(host))
		if err != nil || !sameToken(cookie.Value, token) {
			http.Error(w, "Open the URL printed by agentarch ui to establish your local session.", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/status" {
			v, err := application.Status(root)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(v)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

func sameToken(a, b string) bool {
	return b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func Listen(root string, port int) (net.Listener, *http.Server, string, error) {
	if port < 0 || port > 65535 {
		return nil, nil, "", fmt.Errorf("invalid port")
	}
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, nil, "", err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		_ = l.Close()
		return nil, nil, "", err
	}
	token := hex.EncodeToString(secret[:])
	host := l.Addr().String()
	server := &http.Server{Handler: Handler(root, host, token), ReadHeaderTimeout: 5e9, ReadTimeout: 15e9, WriteTimeout: 30e9, IdleTimeout: 60e9, MaxHeaderBytes: 16 << 10}
	return l, server, "http://" + host + "/?token=" + token, nil
}

// Cookies are shared across ports. Distinct names prevent two project panels on
// localhost from replacing one another's session cookie.
func sessionCookieName(host string) string {
	sum := sha256.Sum256([]byte(host))
	return "agentarch_session_" + hex.EncodeToString(sum[:6])
}
