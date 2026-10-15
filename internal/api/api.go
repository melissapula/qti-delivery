// Package api serves the test form and candidates' sessions over a JSON HTTP
// API.
package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/melissapula/qti-delivery/internal/qti"
	"github.com/melissapula/qti-delivery/internal/session"
)

// server holds what every handler needs. It is safe for concurrent use
// because the store and the logger are.
type server struct {
	store *session.Store
	log   *slog.Logger
}

// New returns the API's HTTP handler over store, logging to log. Paths the
// API does not serve, and wrong methods on paths it does, get ServeMux's own
// plain-text 404 and 405 responses rather than JSON.
func New(store *session.Store, log *slog.Logger) http.Handler {
	s := &server{store: store, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/items", s.listItems)
	mux.HandleFunc("POST /api/sessions", s.startSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("GET /api/sessions/{id}/items/{index}", s.getItem)
	mux.HandleFunc("PUT /api/sessions/{id}/responses/{itemId}", s.putResponse)
	mux.HandleFunc("POST /api/sessions/{id}/submit", s.submit)
	return mux
}

// healthz reports that the server is up.
func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	s.respond(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// itemSummary is one entry in the form's outline. It holds no item content,
// so it cannot leak answers.
type itemSummary struct {
	Index      int     `json:"index"`
	Identifier string  `json:"identifier"`
	Title      string  `json:"title"`
	MaxScore   float64 `json:"maxScore"`
}

// listItems returns the form's outline, in order.
func (s *server) listItems(w http.ResponseWriter, r *http.Request) {
	items := s.store.Items()
	outline := make([]itemSummary, len(items))
	for i, it := range items {
		outline[i] = itemSummary{
			Index:      i,
			Identifier: it.Identifier,
			Title:      it.Title,
			MaxScore:   it.MaxScore(),
		}
	}
	s.respond(w, r, http.StatusOK, outline)
}

// startRequest is the body of a request to start a session.
type startRequest struct {
	CandidateID string `json:"candidateId"`
}

// startSession starts a session for the candidate in the body and returns
// it with 201. A body that cannot be decoded, or has no candidateId, is a
// 400; one over maxBodyBytes is a 413.
func (s *server) startSession(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.writeError(w, r, decodeStatus(err), err.Error())
		return
	}
	if req.CandidateID == "" {
		s.writeError(w, r, http.StatusBadRequest, "candidateId is required")
		return
	}
	sess, err := s.store.Start(req.CandidateID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.respond(w, r, http.StatusCreated, sess)
}

// getSession returns the session, or 404 if there is none.
func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.respond(w, r, http.StatusOK, sess)
}

// itemResponse is one item of the form as the candidate sees it, with its
// place in the form.
type itemResponse struct {
	Index int      `json:"index"`
	Total int      `json:"total"`
	Item  qti.View `json:"item"`
}

// getItem returns the candidate's view of the item at the given index, never
// the item itself, which holds the answers. It is a 404 if the session does
// not exist or the index is out of range, and a 400 if the index is not a
// number. A submitted session can still read items.
func (s *server) getItem(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.Get(r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		s.writeError(w, r, http.StatusBadRequest, "item index must be a number")
		return
	}
	items := s.store.Items()
	if index < 0 || index >= len(items) {
		s.writeError(w, r, http.StatusNotFound, "item not found")
		return
	}
	s.respond(w, r, http.StatusOK, itemResponse{
		Index: index,
		Total: len(items),
		Item:  items[index].View(),
	})
}

// putResponse records the body as the candidate's answer to the item and
// returns 204 with no body. The body maps response identifiers to values, so
// an undeclared identifier is not a decoding error: the store rejects it as
// a bad response, a 422. The statuses are those of fail and decodeStatus.
func (s *server) putResponse(w http.ResponseWriter, r *http.Request) {
	var responses qti.Responses
	if err := decodeJSON(w, r, &responses); err != nil {
		s.writeError(w, r, decodeStatus(err), err.Error())
		return
	}
	if err := s.store.Answer(r.PathValue("id"), r.PathValue("itemId"), responses); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// submit scores and locks the session and returns it with its results. It is
// a 404 if there is no such session and a 409 if it is already submitted.
func (s *server) submit(w http.ResponseWriter, r *http.Request) {
	sess, err := s.store.Submit(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.respond(w, r, http.StatusOK, sess)
}
