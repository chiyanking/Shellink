package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"shellink/internal/config"
	"shellink/internal/protocol"
	"shellink/internal/rpc"
	"shellink/internal/service"
	"shellink/internal/sessions"
	"shellink/internal/webhooks"
)

type Server struct {
	cfg      config.Config
	rpc      *rpc.Dispatcher
	sessions *sessions.Manager
	webhooks *webhooks.Service
	bus      *sessions.Bus
	assets   http.Handler
}

func New(cfg config.Config, dispatcher *rpc.Dispatcher, manager *sessions.Manager, hooks *webhooks.Service, bus *sessions.Bus) *Server {
	return &Server{cfg: cfg, rpc: dispatcher, sessions: manager, webhooks: hooks, bus: bus, assets: uiAssets()}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /shellink/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "name": "Shellink"})
	})
	mux.HandleFunc("GET /shellink/api/auth/sensitive-ops", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"requireToken": s.sensitiveRequired(r)})
	})
	mux.HandleFunc("GET /shellink/api/profiles", s.profilesList)
	mux.HandleFunc("POST /shellink/api/profiles", s.profileCreate)
	mux.HandleFunc("GET /shellink/api/profiles/{id}", s.profileGet)
	mux.HandleFunc("PUT /shellink/api/profiles/{id}", s.profileUpdate)
	mux.HandleFunc("DELETE /shellink/api/profiles/{id}", s.profileDelete)
	mux.HandleFunc("GET /shellink/api/sessions", s.sessionsList)
	mux.HandleFunc("POST /shellink/api/sessions", s.sessionCreate)
	mux.HandleFunc("DELETE /shellink/api/sessions/records", s.sessionPurge)
	mux.HandleFunc("GET /shellink/api/sessions/{id}/state", s.sessionState)
	mux.HandleFunc("GET /shellink/api/sessions/{id}/history", s.sessionHistory)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/input", s.sessionInput)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/exec", s.sessionExec)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/exec-start", s.sessionExecStart)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/exec-status", s.sessionExecStatus)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/exec-cancel", s.sessionExecCancel)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/upload-start", s.sessionUploadStart)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/download-start", s.sessionDownloadStart)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/edit-start", s.sessionEditStart)
	mux.HandleFunc("GET /shellink/api/sessions/{id}/download", s.sessionDownload)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/upload", s.sessionUpload)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/edit", s.sessionEdit)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/mode", s.sessionMode)
	mux.HandleFunc("POST /shellink/api/sessions/{id}/close", s.sessionClose)
	mux.HandleFunc("DELETE /shellink/api/sessions/{id}/record", s.sessionRemove)
	mux.HandleFunc("DELETE /shellink/api/sessions/{id}", s.sessionRemove)
	mux.HandleFunc("GET /shellink/api/webhooks", s.webhookList)
	mux.HandleFunc("POST /shellink/api/webhooks", s.webhookCreate)
	mux.HandleFunc("DELETE /shellink/api/webhooks/{id}", s.webhookDelete)
	mux.HandleFunc("GET /shellink/api/webhook-messages", s.webhookMessages)
	mux.HandleFunc("DELETE /shellink/api/webhook-messages", s.webhookMessagesClear)
	mux.HandleFunc("POST /shellink/webhook/callback", s.webhookCallback)
	mux.HandleFunc("GET /shellink/agent.md", s.agentDoc)
	mux.HandleFunc("GET /shellink/llms.txt", s.agentDoc)
	mux.HandleFunc("GET /shellink/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/shellink/ui/", http.StatusTemporaryRedirect)
	})
	mux.Handle("/shellink/ui/", http.StripPrefix("/shellink/ui/", s.assets))
	mux.HandleFunc("GET /shellink/ws/sessions/{id}", s.terminalWebSocket)
	mux.HandleFunc("GET /shellink/ws/events", s.eventsWebSocket)
	return s.cors(s.auth(mux))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/shellink/api/") && path != "/shellink/api/auth/sensitive-ops" && !s.localRequest(r) && !s.validBearer(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized: invalid Bearer token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) localRequest(r *http.Request) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remote = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimPrefix(strings.ToLower(remote), "::ffff:"))
	if ip == nil || !ip.IsLoopback() || !localHost(r.Host) {
		return false
	}
	return true
}

func localHost(host string) bool {
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	} else if strings.HasPrefix(host, "[") {
		if end := strings.IndexByte(host, ']'); end >= 0 {
			host = host[1:end]
		}
	} else if strings.Count(host, ":") == 1 {
		host = strings.SplitN(host, ":", 2)[0]
	}
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func (s *Server) validBearer(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	return subtleTokenEqual(strings.TrimPrefix(header, "Bearer "), s.cfg.Token)
}

func subtleTokenEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := range left {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}

func (s *Server) sensitiveRequired(r *http.Request) bool {
	if s.cfg.RequireTokenForSensitive != nil {
		return *s.cfg.RequireTokenForSensitive
	}
	return !s.localRequest(r)
}

func (s *Server) requireSensitive(w http.ResponseWriter, r *http.Request, message string) bool {
	if !s.sensitiveRequired(r) || s.validBearer(r) {
		return true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized: " + message + " requires a valid Bearer token"})
	return false
}

func (s *Server) profilesList(w http.ResponseWriter, r *http.Request) {
	value, err := s.rpc.Dispatch("profiles.list", protocol.Object("q", r.URL.Query().Get("q")), nil)
	s.respond(w, value, err)
}

func (s *Server) profileGet(w http.ResponseWriter, r *http.Request) {
	s.dispatch(w, "profiles.get", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) profileCreate(w http.ResponseWriter, r *http.Request) {
	body, err := readJSON(r, 2<<20)
	if err != nil {
		s.respondError(w, service.Invalid(err.Error()))
		return
	}
	value, err := s.rpc.Dispatch("profiles.create", protocol.JSONValue(body), nil)
	s.respondStatus(w, http.StatusCreated, value, err)
}

func (s *Server) profileUpdate(w http.ResponseWriter, r *http.Request) {
	body, err := readJSON(r, 2<<20)
	if err != nil {
		s.respondError(w, service.Invalid(err.Error()))
		return
	}
	s.dispatch(w, "profiles.update", map[string]any{"id": r.PathValue("id"), "profile": body})
}

func (s *Server) profileDelete(w http.ResponseWriter, r *http.Request) {
	value, err := s.rpc.Dispatch("profiles.delete", protocol.JSONValue(map[string]any{"id": r.PathValue("id")}), nil)
	if err != nil {
		s.respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	_ = value
}

func (s *Server) sessionsList(w http.ResponseWriter, _ *http.Request) {
	s.dispatch(w, "sessions.list", map[string]any{})
}

func (s *Server) sessionCreate(w http.ResponseWriter, r *http.Request) {
	body, err := readJSON(r, 2<<20)
	if err != nil {
		s.respondError(w, service.Invalid(err.Error()))
		return
	}
	value, err := s.rpc.Dispatch("sessions.create", protocol.JSONValue(body), nil)
	s.respondStatus(w, http.StatusCreated, value, err)
}

func (s *Server) sessionState(w http.ResponseWriter, r *http.Request) {
	s.dispatch(w, "sessions.state", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	since, err1 := queryInt(query.Get("since"), 0)
	limit, err2 := queryInt(query.Get("limit"), 2000)
	if err1 != nil || err2 != nil {
		s.respondError(w, service.Invalid("since and limit must be integers"))
		return
	}
	include := query.Get("includeInternal") == "1" || strings.EqualFold(query.Get("includeInternal"), "true")
	s.dispatch(w, "sessions.history", map[string]any{"id": r.PathValue("id"), "since": since, "limit": limit, "includeInternal": include})
}

func (s *Server) sessionInput(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.input", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionExec(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.exec", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionExecStart(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.execStart", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionExecStatus(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.execStatus", map[string]any{})
}

func (s *Server) sessionExecCancel(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.execCancel", map[string]any{})
}

func (s *Server) sessionUploadStart(w http.ResponseWriter, r *http.Request) {
	file, header, err := s.multipartFile(w, r)
	if err != nil {
		s.respondError(w, err)
		return
	}
	query := r.URL.Query()
	params := map[string]any{"id": r.PathValue("id"), "path": query.Get("path"), "data": file, "timeoutMs": queryIntDefault(query.Get("timeoutMs"), s.cfg.TransferTimeoutMS)}
	if hash := strings.TrimSpace(query.Get("sha256")); hash != "" {
		params["sha256"] = hash
	}
	_ = header
	s.dispatch(w, "sessions.uploadStart", params)
}

func (s *Server) sessionDownloadStart(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.downloadStart", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionEditStart(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.editStart", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionDownload(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{"id": r.PathValue("id"), "path": r.URL.Query().Get("path"), "timeoutMs": queryIntDefault(r.URL.Query().Get("timeoutMs"), s.cfg.TransferTimeoutMS)}
	value, err := s.rpc.Dispatch("sessions.download", protocol.JSONValue(params), nil)
	if err != nil {
		s.respondError(w, err)
		return
	}
	result := protocol.MapValue(value)
	data, _ := result["data"].([]byte)
	remotePath, _ := protocol.StringValue(result["remotePath"])
	size, _ := protocol.IntValue(result["size"])
	hash, _ := protocol.StringValue(result["sha256"])
	codec, _ := protocol.StringValue(result["codec"])
	duration, _ := protocol.IntValue(result["durationMs"])
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(remotePath)+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("X-Shellink-Path", url.QueryEscape(remotePath))
	w.Header().Set("X-Shellink-Size", strconv.FormatInt(size, 10))
	w.Header().Set("X-Shellink-SHA256", hash)
	w.Header().Set("X-Shellink-Codec", codec)
	w.Header().Set("X-Shellink-Duration-Ms", strconv.FormatInt(duration, 10))
	_, _ = w.Write(data)
}

func (s *Server) sessionUpload(w http.ResponseWriter, r *http.Request) {
	file, _, err := s.multipartFile(w, r)
	if err != nil {
		s.respondError(w, err)
		return
	}
	query := r.URL.Query()
	params := map[string]any{"id": r.PathValue("id"), "path": query.Get("path"), "data": file, "timeoutMs": queryIntDefault(query.Get("timeoutMs"), s.cfg.TransferTimeoutMS)}
	if hash := strings.TrimSpace(query.Get("sha256")); hash != "" {
		params["sha256"] = hash
	}
	s.dispatch(w, "sessions.upload", params)
}

func (s *Server) sessionEdit(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.edit", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionMode(w http.ResponseWriter, r *http.Request) {
	s.dispatchBody(w, r, "sessions.mode", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionClose(w http.ResponseWriter, r *http.Request) {
	s.dispatch(w, "sessions.close", map[string]any{"id": r.PathValue("id")})
}

func (s *Server) sessionRemove(w http.ResponseWriter, r *http.Request) {
	if !s.requireSensitive(w, r, "deleting a session record") {
		return
	}
	value, err := s.rpc.Dispatch("sessions.removeRecord", protocol.JSONValue(map[string]any{"id": r.PathValue("id")}), nil)
	if err != nil {
		s.respondError(w, err)
		return
	}
	_ = value
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sessionPurge(w http.ResponseWriter, r *http.Request) {
	if !s.requireSensitive(w, r, "purging session records") {
		return
	}
	olderThan := r.URL.Query().Get("olderThan")
	if olderThan == "" {
		body, err := readJSON(r, 1<<20)
		if err == nil {
			if value, ok := body["olderThan"].(string); ok {
				olderThan = value
			}
		}
	}
	value, err := s.sessions.Purge(olderThan)
	s.respond(w, value, err)
}

func (s *Server) webhookList(w http.ResponseWriter, _ *http.Request) {
	s.dispatch(w, "webhooks.list", map[string]any{})
}

func (s *Server) webhookCreate(w http.ResponseWriter, r *http.Request) {
	s.dispatchBodyStatus(w, r, "webhooks.create", map[string]any{}, http.StatusCreated)
}

func (s *Server) webhookDelete(w http.ResponseWriter, r *http.Request) {
	value, err := s.rpc.Dispatch("webhooks.delete", protocol.JSONValue(map[string]any{"id": r.PathValue("id")}), nil)
	if err != nil {
		s.respondError(w, err)
		return
	}
	_ = value
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) webhookMessages(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, s.webhooks.Messages())
}
func (s *Server) webhookMessagesClear(w http.ResponseWriter, _ *http.Request) {
	s.webhooks.ClearMessages()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) webhookCallback(w http.ResponseWriter, r *http.Request) {
	var data any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20)).Decode(&data); err != nil {
		s.respondError(w, service.Invalid("Invalid JSON body"))
		return
	}
	message := s.webhooks.Receive(data)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": message.ID})
}

func (s *Server) agentDoc(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = io.WriteString(w, AgentDoc)
}

func (s *Server) dispatch(w http.ResponseWriter, method string, params map[string]any) {
	value, err := s.rpc.Dispatch(method, protocol.JSONValue(params), nil)
	s.respond(w, value, err)
}

func (s *Server) dispatchBody(w http.ResponseWriter, r *http.Request, method string, pathParams map[string]any) {
	s.dispatchBodyStatus(w, r, method, pathParams, http.StatusOK)
}

func (s *Server) dispatchBodyStatus(w http.ResponseWriter, r *http.Request, method string, pathParams map[string]any, status int) {
	body, err := readJSON(r, 20<<20)
	if err != nil {
		s.respondError(w, service.Invalid(err.Error()))
		return
	}
	for key, value := range body {
		pathParams[key] = value
	}
	value, err := s.rpc.Dispatch(method, protocol.JSONValue(pathParams), nil)
	s.respondStatus(w, status, value, err)
}

func (s *Server) respond(w http.ResponseWriter, value protocol.Value, err error) {
	s.respondStatus(w, http.StatusOK, value, err)
}

func (s *Server) respondStatus(w http.ResponseWriter, status int, value protocol.Value, err error) {
	if err != nil {
		s.respondError(w, err)
		return
	}
	writeJSON(w, status, protocol.ToJSONValue(value))
}

func (s *Server) respondError(w http.ResponseWriter, err error) {
	var app *service.AppError
	if !errors.As(err, &app) {
		app = service.Internal(err)
	}
	body := map[string]any{"error": app.Message}
	if app.Details != nil {
		body["details"] = app.Details
	}
	writeJSON(w, app.Status, body)
}

func (s *Server) multipartFile(w http.ResponseWriter, r *http.Request) ([]byte, string, *service.AppError) {
	if r.URL.Query().Get("path") == "" {
		return nil, "", service.Invalid("Missing query parameter: path")
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(s.cfg.TransferMaxBytes)+(1<<20))
	if err := r.ParseMultipartForm(int64(s.cfg.TransferMaxBytes)); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			return nil, "", &service.AppError{Code: "PAYLOAD_TOO_LARGE", Message: fmt.Sprintf("File is too large; limit is %d bytes", s.cfg.TransferMaxBytes), Status: 413}
		}
		return nil, "", service.Invalid(err.Error())
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", service.Invalid("Missing multipart field: file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(s.cfg.TransferMaxBytes)+1))
	if err != nil {
		return nil, "", service.Internal(err)
	}
	if len(data) > s.cfg.TransferMaxBytes {
		return nil, "", &service.AppError{Code: "PAYLOAD_TOO_LARGE", Message: fmt.Sprintf("File is too large; limit is %d bytes", s.cfg.TransferMaxBytes), Status: 413}
	}
	return data, header.Filename, nil
}

func readJSON(r *http.Request, maxBytes int64) (map[string]any, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxBytes+1))
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("JSON body must be an object")
	}
	return body, nil
}

func queryInt(raw string, fallback int64) (int64, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func queryIntDefault(raw string, fallback int) int {
	value, err := queryInt(raw, int64(fallback))
	if err != nil {
		return -1
	}
	return int(value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func uiAssets() http.Handler {
	if embedded, ok := embeddedUIAssets(); ok {
		return embedded
	}
	candidates := []string{"web/dist", "../web/dist", filepath.Join(filepath.Dir(os.Args[0]), "web", "dist")}
	if executable, err := exec.LookPath(os.Args[0]); err == nil {
		if abs, err := filepath.Abs(executable); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(abs), "web", "dist"))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			root := http.Dir(candidate)
			files := http.FileServer(root)
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				clean := filepath.Clean("/" + r.URL.Path)
				if file, err := root.Open(clean); err == nil {
					file.Close()
					files.ServeHTTP(w, r)
					return
				}
				if filepath.Ext(clean) != "" {
					http.NotFound(w, r)
					return
				}
				http.ServeFile(w, r, filepath.Join(candidate, "index.html"))
			})
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Shellink Web UI assets were not found", http.StatusNotFound)
	})
}
