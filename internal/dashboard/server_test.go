package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadOnlyLocalSessionBoundary(t *testing.T) {
	root := t.TempDir()
	h := Handler(root, "127.0.0.1:4321", "test-session")
	for _, tc := range []struct {
		name, method, path, host, origin, cookie string
		want                                     int
	}{
		{"bootstrap", "GET", "/?token=test-session", "127.0.0.1:4321", "", "", 303},
		{"anonymous", "GET", "/api/status", "127.0.0.1:4321", "", "", 401},
		{"read", "GET", "/api/status", "127.0.0.1:4321", "", "test-session", 200},
		{"write", "POST", "/api/status", "127.0.0.1:4321", "", "test-session", 405},
		{"dns-rebind", "GET", "/api/status", "attacker.test", "", "test-session", 403},
		{"cross-origin", "GET", "/api/status", "127.0.0.1:4321", "https://attacker.test", "test-session", 403},
		{"wrong-token", "GET", "/?token=no", "127.0.0.1:4321", "", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionCookieName("127.0.0.1:4321"), Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		})
	}
}

func TestSeparatePanelsDoNotOverwriteSessionCookies(t *testing.T) {
	cookies := []*http.Cookie{}
	for _, host := range []string{"127.0.0.1:4321", "127.0.0.1:4322"} {
		h := Handler(t.TempDir(), host, "session-token")
		r := httptest.NewRequest("GET", "/?token=session-token", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		cs := w.Result().Cookies()
		if len(cs) != 1 {
			t.Fatal("missing bootstrap cookie")
		}
		cookies = append(cookies, cs[0])
	}
	if cookies[0].Name == cookies[1].Name {
		t.Fatal("project panels overwrite one another's authentication")
	}
	for _, c := range cookies {
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
			t.Fatal("cookie protection regressed")
		}
	}
}
