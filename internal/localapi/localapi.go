// Package localapi serves the daemon state to the local dashboard on port 9090.
// The GUI (or a plain browser during development) reads live data from here, so
// the window is a pure viewer over the daemon.
//
// It listens on loopback only and answers only this app's own window or a
// local development server: a web page on another site could otherwise use a
// contributor's browser to change the node's settings, for example pointing it
// at a different Coordinator.
package localapi

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/daemon"
)

// Server exposes the daemon over HTTP.
type Server struct {
	daemon *daemon.Daemon
	addr   string
}

// DefaultAddr is loopback only, so nothing else on the network can reach it.
const DefaultAddr = "127.0.0.1:9090"

// New builds a local API server bound to the given address. A bare port such as
// ":9090" is bound to loopback rather than every interface.
func New(d *daemon.Daemon, addr string) *Server {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	return &Server{daemon: d, addr: addr}
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/enroll", s.handleEnroll)
	mux.HandleFunc("/api/update", s.handleUpdate)
	mux.HandleFunc("/api/cache", s.handleCache)
	return localOnly(mux)
}

type enrollRequest struct {
	Code string `json:"code"`
}

// handleEnroll exchanges an enrollment code from Zonn Console for this node's
// credential. The credential itself never leaves the daemon.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req enrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.daemon.Enroll(ctx, req.Code); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.daemon.Snapshot())
}

// handleUpdate acts on a newer Zonn Node: "check" asks the backend now, and
// "install" downloads the installer, checks its SHA-256, and opens it.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	switch req.Action {
	case "check":
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		s.daemon.CheckUpdate(ctx)
	case "install":
		if err := s.daemon.InstallUpdate(); err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be check or install"})
		return
	}
	writeJSON(w, http.StatusOK, s.daemon.Snapshot().Update)
}

// handleCache shows the model cache (GET) or clears it (POST with action
// "clear"), keeping every model a running stage needs.
func (s *Server) handleCache(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action != "clear" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be clear"})
			return
		}
		s.daemon.ClearCache()
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.daemon.Snapshot().ModelCache)
}

// ListenAndServe starts the server and blocks.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.daemon.Snapshot())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.daemon.Config())
	case http.MethodPost:
		var cfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid config body"})
			return
		}
		if err := s.daemon.UpdateConfig(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.daemon.Config())
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type reportRequest struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	level := strings.ToUpper(strings.TrimSpace(req.Level))
	if level == "" {
		level = "ERROR"
	}
	s.daemon.PushError(level, req.Message)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reported"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("localapi: encode: %v", err)
	}
}

// localOnly admits requests only from this machine's own app. The Host header
// must name loopback, which defeats DNS rebinding, and a browser Origin must be
// the app's webview or a local development server. Requests with no Origin
// come from local programs such as curl, which can read the config file anyway.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !allowedOrigin(origin) {
				log.Printf("localapi: refused a request from origin %q", origin)
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// allowedOrigin accepts the Wails webview (wails://wails on macOS and Linux,
// http://wails.localhost on Windows) and local development servers.
func allowedOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch {
	case u.Scheme == "wails" && u.Host == "wails":
		return true
	case (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() == "wails.localhost":
		return true
	case u.Scheme == "http" && loopbackHost(u.Host):
		return true
	}
	return false
}
