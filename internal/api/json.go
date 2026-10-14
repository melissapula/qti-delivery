package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/melissapula/qti-delivery/internal/session"
)

// maxBodyBytes caps the size of a request body at 1 MiB.
const maxBodyBytes = 1 << 20

var (
	errEmptyBody    = errors.New("empty body")
	errTrailingData = errors.New("body must hold a single JSON value")
)

// errorBody is the JSON shape of every error the API writes itself.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON writes v as JSON with the given status. The status is sent
// before v is encoded, so an encoding error can only be returned for the
// caller to log.
func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// respond writes v as JSON with the given status and logs any encoding error.
func (s *server) respond(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := writeJSON(w, status, v); err != nil {
		s.log.Error("write response", "method", r.Method, "path", r.URL.Path, "err", err)
	}
}

// writeError writes msg as a JSON error with the given status.
func (s *server) writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	s.respond(w, r, status, errorBody{Error: msg})
}

// decodeJSON decodes the request body into dst. The body must hold exactly
// one JSON value and be no larger than maxBodyBytes; when dst is a struct, a
// field it does not have is an error. Pass any error to decodeStatus for the
// response status.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errEmptyBody
		}
		return err
	}

	// Anything but the end of the body after the value is an error. A body
	// too large to read to the end keeps its own error, so it reports 413.
	err := dec.Decode(&struct{}{})
	switch {
	case errors.Is(err, io.EOF):
		return nil
	case isTooLarge(err):
		return err
	default:
		return errTrailingData
	}
}

// isTooLarge reports whether err comes from reading past maxBodyBytes.
func isTooLarge(err error) bool {
	_, ok := errors.AsType[*http.MaxBytesError](err)
	return ok
}

// decodeStatus returns the HTTP status for an error from decodeJSON: 413 for
// a body over maxBodyBytes and 400 for anything else.
func decodeStatus(err error) int {
	if isTooLarge(err) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// statusFor returns the HTTP status for an error from the session store: 404
// when the session or item does not exist, 409 when the session is already
// submitted, 422 when the answer does not fit the item, and 500 for anything
// else. Wrapped errors map the same as the error they wrap.
func statusFor(err error) int {
	switch {
	case errors.Is(err, session.ErrNotFound), errors.Is(err, session.ErrUnknownItem):
		return http.StatusNotFound
	case errors.Is(err, session.ErrAlreadyDone):
		return http.StatusConflict
	case errors.Is(err, session.ErrBadResponse):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// fail writes the response for err, an error from the session store. A 500
// is logged with the real error, and the client is told only "internal error"
// so that internals do not leak.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := statusFor(err)
	if status == http.StatusInternalServerError {
		s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		s.writeError(w, r, status, "internal error")
		return
	}
	s.writeError(w, r, status, err.Error())
}
