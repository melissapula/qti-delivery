package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logLines decodes each line a JSON slog handler wrote to buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

// onlyRequestLine checks that buf holds exactly one "request" log line at
// INFO, with a non-negative duration, and returns it.
func onlyRequestLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := logLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1:\n%s", len(lines), buf)
	}
	line := lines[0]
	if line["msg"] != "request" || line["level"] != "INFO" {
		t.Errorf("log line = %v, want msg request at INFO", line)
	}
	if d, ok := line["duration"].(float64); !ok || d < 0 {
		t.Errorf("duration = %v, want a non-negative number", line["duration"])
	}
	return line
}

// TestLogRequestsStatus checks that the logged status is the one the client
// receives, however the handler sets it.
func TestLogRequestsStatus(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{
			name:    "body only is 200",
			handler: func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hi")) },
			want:    http.StatusOK,
		},
		{
			name:    "nothing written is 200",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			want:    http.StatusOK,
		},
		{
			name:    "explicit status",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) },
			want:    http.StatusNotFound,
		},
		{
			name: "status then body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte("made"))
			},
			want: http.StatusCreated,
		},
		{
			// Writing the body sends a 200, so the later status is ignored.
			name: "status after body ignored",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("hi"))
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusOK,
		},
		{
			// net/http ignores every WriteHeader after the first.
			name: "second status ignored",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, nil))
			rec := httptest.NewRecorder()

			logRequests(log, tt.handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

			if rec.Code != tt.want {
				t.Errorf("client got status %d, want %d", rec.Code, tt.want)
			}
			line := onlyRequestLine(t, &buf)
			if line["status"] != float64(tt.want) {
				t.Errorf("logged status = %v, want %d", line["status"], tt.want)
			}
		})
	}
}

// TestStatusRecorderUnwrap checks that http.ResponseController reaches the
// writer under the recorder, so wrapping does not hide its features.
func TestStatusRecorderUnwrap(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if err := http.NewResponseController(rec).Flush(); err != nil {
		t.Errorf("Flush through the recorder: %v", err)
	}
}

// TestLogRequests checks that New logs every request with its method, path
// and status.
func TestLogRequests(t *testing.T) {
	tests := []struct {
		name         string
		method, path string
		body         string
		wantStatus   int
	}{
		{"start session", http.MethodPost, "/api/sessions", `{"candidateId":"c-123"}`, http.StatusCreated},
		{"unknown session", http.MethodGet, "/api/sessions/nope", "", http.StatusNotFound},
		{"unserved path", http.MethodGet, "/nope", "", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := New(newTestStore(t), slog.New(slog.NewJSONHandler(&buf, nil)))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))

			line := onlyRequestLine(t, &buf)
			if line["method"] != tt.method || line["path"] != tt.path || line["status"] != float64(tt.wantStatus) {
				t.Errorf("logged method %v path %v status %v, want %s %s %d",
					line["method"], line["path"], line["status"], tt.method, tt.path, tt.wantStatus)
			}
		})
	}
}
