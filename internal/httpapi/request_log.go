package httpapi

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"time"
)

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &requestLogWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		status := wrapped.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("[Shellink HTTP] method=%s path=%s status=%d duration=%s remote=%s", r.Method, r.URL.Path, status, time.Since(started).Round(time.Millisecond), r.RemoteAddr)
	})
}

type requestLogWriter struct {
	http.ResponseWriter
	status int
}

func (w *requestLogWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *requestLogWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *requestLogWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *requestLogWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *requestLogWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.status = http.StatusSwitchingProtocols
	return hijacker.Hijack()
}
