package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder is a ResponseWriter that remembers the status sent to the
// client, so the logging middleware can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int // 0 until the handler writes a header or a body
}

// WriteHeader records the first status, the one net/http sends, then passes
// the call on.
func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

// Write records a 200 if no status was set, as net/http sends one, then
// passes the call on.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap returns the wrapped writer, so http.ResponseController can reach
// features such as Flush that the recorder does not declare.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// logRequests wraps next so every request is logged at Info, after it is
// served, with its method, path, status and duration.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			// The handler wrote nothing, so net/http sends a 200.
			rec.status = http.StatusOK
		}
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start))
	})
}
