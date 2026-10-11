package localapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlyTheAppItselfMayCallTheLocalAPI(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := localOnly(ok)
	cases := []struct {
		name, host, origin string
		want               int
	}{
		{"desktop window on macOS and Linux", "localhost:9090", "wails://wails", 200},
		{"desktop window on Windows", "localhost:9090", "http://wails.localhost", 200},
		{"desktop window on Windows with a port", "localhost:9090", "http://wails.localhost:34115", 200},
		{"vite dev server", "localhost:9090", "http://localhost:5173", 200},
		{"local program without an origin", "127.0.0.1:9090", "", 200},
		{"another website", "localhost:9090", "https://evil.example", 403},
		{"a lookalike host", "localhost:9090", "http://wails.localhost.evil.example", 403},
		{"a remote http origin", "localhost:9090", "http://192.168.1.20:8000", 403},
		{"DNS rebinding", "evil.example:9090", "http://evil.example:9090", 403},
		{"DNS rebinding without an origin", "evil.example:9090", "", 403},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "http://"+c.host+"/api/config", nil)
		req.Host = c.host
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
		if c.want == 200 && c.origin != "" && rec.Header().Get("Access-Control-Allow-Origin") != c.origin {
			t.Errorf("%s: CORS header %q", c.name, rec.Header().Get("Access-Control-Allow-Origin"))
		}
		if c.want == 403 && rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: a refused origin got a CORS header", c.name)
		}
	}
}

func TestBarePortBindsToLoopback(t *testing.T) {
	if s := New(nil, ":9090"); s.addr != "127.0.0.1:9090" {
		t.Fatalf("bound to %s", s.addr)
	}
}
