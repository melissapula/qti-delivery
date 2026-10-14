// Package api serves the test form and candidates' sessions over a JSON HTTP
// API.
package api

import (
	"log/slog"
	"net/http"

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
