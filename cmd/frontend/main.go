package main

import (
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"VLX_ChatBridge/internal/ui"
)

func main() {
	cfgPath := "config/frontend.settings"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg := loadFrontendConfig(cfgPath)

	backend := "http://" + cfg.BackendAddr + ":" + cfg.BackendPort
	target, err := url.Parse(backend)
	if err != nil {
		log.Fatalf("invalid backend URL %q: %v", backend, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Inject the backend control-API credential so the browser only ever needs
	// the GUI credential. The backend binds to localhost; this bridges the two.
	backendAuth := ""
	if cfg.BackendUser != "" {
		backendAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.BackendUser+":"+cfg.BackendPass))
	}
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		if backendAuth != "" {
			r.Header.Set("Authorization", backendAuth)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/api/", proxy) // REST + console WebSocket, both forwarded to the backend
	mux.Handle("/", ui.Handler())

	handler := withGUIAuth(cfg.GUIUser, cfg.GUIPass, mux)

	bind := cfg.BindAddr + ":" + cfg.BindPort
	log.Printf("VLX_ChatBridge frontend listening on %s -> backend %s", bind, backend)
	if err := http.ListenAndServe(bind, handler); err != nil {
		log.Fatalf("frontend server failed: %v", err)
	}
}

// withGUIAuth enforces BasicAuth for the GUI. It exempts the health check and
// the console WebSocket: the latter is authorized by a single-use ticket that
// the backend validates, because browsers cannot set an Authorization header on
// a WebSocket handshake. If no GUI user is configured, auth is skipped entirely.
func withGUIAuth(user, pass string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user == "" || r.URL.Path == "/health" || r.URL.Path == "/api/console/ws" {
			next.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="VLX_ChatBridge GUI"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
