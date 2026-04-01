package inspect

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

//go:embed assets/*
var assets embed.FS

var readAssetFile = fs.ReadFile

type App struct {
	store    *Store
	username string
	password string
}

func NewApp(directory string) *App {
	return &App{store: NewStore(directory)}
}

func (a *App) SetBasicAuth(username, password string) {
	if a == nil {
		return
	}

	a.username = strings.TrimSpace(username)
	a.password = password
}

func (a *App) Validate() error {
	if a == nil || a.store == nil {
		return errors.New("trace app is required")
	}

	return a.store.Validate()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", a.handleSessions)
	mux.HandleFunc("/api/sessions/", a.handleSession)
	mux.HandleFunc("/", a.handleIndex)
	if !a.requiresAuth() {
		return mux
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authorized(r) {
			mux.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="Hand Trace Viewer", charset="UTF-8"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}

func (a *App) requiresAuth() bool {
	if a == nil {
		return false
	}

	return a.username != "" || a.password != ""
}

func (a *App) authorized(r *http.Request) bool {
	if a == nil || !a.requiresAuth() {
		return true
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}

	if subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) != 1 {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
}

func (a *App) handleIndex(w http.ResponseWriter, _ *http.Request) {
	content, err := readAssetFile(assets, "assets/index.html")
	if err != nil {
		http.Error(w, "failed to load trace viewer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}

func (a *App) handleSessions(w http.ResponseWriter, _ *http.Request) {
	sessions, err := a.store.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/sessions/"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := a.store.GetSession(id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, fs.ErrPermission) {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": strings.TrimSpace(message)})
}
