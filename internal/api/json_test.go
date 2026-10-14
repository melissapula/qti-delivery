package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/melissapula/qti-delivery/internal/session"
)

func TestStatusFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", session.ErrNotFound, http.StatusNotFound},
		{"unknown item", session.ErrUnknownItem, http.StatusNotFound},
		{"already submitted", session.ErrAlreadyDone, http.StatusConflict},
		{"bad response", session.ErrBadResponse, http.StatusUnprocessableEntity},
		{"wrapped not found", fmt.Errorf("x: %w", session.ErrNotFound), http.StatusNotFound},
		{"wrapped unknown item", fmt.Errorf("x: %w", session.ErrUnknownItem), http.StatusNotFound},
		{"wrapped already submitted", fmt.Errorf("x: %w", session.ErrAlreadyDone), http.StatusConflict},
		{
			// The way Answer wraps it: the sentinel first, then the detail.
			name: "bad response wrapped as Answer does",
			err:  fmt.Errorf("%w: %v", session.ErrBadResponse, errors.New(`unknown response "X"`)),
			want: http.StatusUnprocessableEntity,
		},
		{"unknown error", errors.New("disk on fire"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusFor(tt.err); got != tt.want {
				t.Errorf("statusFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// sample is a request body type for testing decodeJSON.
type sample struct {
	Name string `json:"name"`
}

// sizedBody returns a valid sample body exactly n bytes long.
func sizedBody(n int) string {
	const prefix, suffix = `{"name":"`, `"}`
	return prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		// want is the decoded name when the body is accepted.
		want string
		// wantStatus is decodeStatus of the error, or 0 when the body is
		// accepted.
		wantStatus int
		// wantMsg, if set, must appear in the error.
		wantMsg string
	}{
		{name: "valid", body: `{"name":"a"}`, want: "a"},
		{name: "trailing whitespace", body: "{\"name\":\"a\"}\n", want: "a"},
		{name: "exactly the cap", body: sizedBody(maxBodyBytes), want: strings.Repeat("a", maxBodyBytes-11)},
		{name: "one byte over the cap", body: sizedBody(maxBodyBytes + 1), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "empty", body: "", wantStatus: http.StatusBadRequest, wantMsg: "empty body"},
		{name: "only whitespace", body: " \n", wantStatus: http.StatusBadRequest, wantMsg: "empty body"},
		{name: "malformed", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"name":"a","extra":1}`, wantStatus: http.StatusBadRequest},
		{name: "wrong type", body: `{"name":1}`, wantStatus: http.StatusBadRequest},
		{name: "second value", body: `{"name":"a"}{"name":"b"}`, wantStatus: http.StatusBadRequest},
		{name: "trailing garbage", body: `{"name":"a"} x`, wantStatus: http.StatusBadRequest},
		{
			// The value fits; the junk after it pushes the body over the cap.
			name:       "over the cap after a valid value",
			body:       `{"name":"a"}` + strings.Repeat(" ", maxBodyBytes),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var got sample
			err := decodeJSON(httptest.NewRecorder(), r, &got)

			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("decodeJSON: %v", err)
				}
				if got.Name != tt.want {
					t.Errorf("decoded name has length %d, want %d", len(got.Name), len(tt.want))
				}
				return
			}
			if err == nil {
				t.Fatalf("decodeJSON accepted the body, want status %d", tt.wantStatus)
			}
			if status := decodeStatus(err); status != tt.wantStatus {
				t.Errorf("decodeStatus(%v) = %d, want %d", err, status, tt.wantStatus)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

// errorMessage checks that rec holds a JSON error response and returns its
// message.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not a JSON error: %v", rec.Body, err)
	}
	return body.Error
}

func TestFailLogsInternalError(t *testing.T) {
	var logs bytes.Buffer
	s := &server{log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()

	s.fail(rec, httptest.NewRequest(http.MethodPost, "/api/sessions/abc/submit", nil), errors.New("disk on fire"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if msg := errorMessage(t, rec); msg != "internal error" {
		t.Errorf("message = %q, want %q", msg, "internal error")
	}
	if !strings.Contains(logs.String(), "disk on fire") {
		t.Errorf("log = %q, want it to contain the real error", logs.String())
	}
}

func TestFailReportsClientError(t *testing.T) {
	var logs bytes.Buffer
	s := &server{log: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()

	s.fail(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/abc", nil), session.ErrNotFound)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if msg := errorMessage(t, rec); msg != "session not found" {
		t.Errorf("message = %q, want %q", msg, "session not found")
	}
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("log = %q, want no error logged for a client error", logs.String())
	}
}
