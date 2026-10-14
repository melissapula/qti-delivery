// Package session tracks candidates' attempts at the test form: starting an
// attempt, recording answers, and scoring it on submit.
package session

import (
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/melissapula/qti-delivery/internal/qti"
)

// Status is where a session is in its life.
type Status string

// The states a session moves through.
const (
	// StatusInProgress means the candidate can still answer.
	StatusInProgress Status = "in_progress"
	// StatusSubmitted means the session is scored and locked.
	StatusSubmitted Status = "submitted"
)

// Session is one candidate's attempt at the test form. It does not include
// the candidate's answers.
type Session struct {
	ID          string    `json:"id"`
	CandidateID string    `json:"candidateId"`
	Status      Status    `json:"status"`
	StartedAt   time.Time `json:"startedAt"`
	SubmittedAt time.Time `json:"submittedAt,omitzero"`

	// Results, Score and MaxScore are set by submitting the session.
	Results  []qti.Result `json:"results,omitempty"`
	Score    float64      `json:"score"`
	MaxScore float64      `json:"maxScore"`
}

var (
	// ErrNotFound is returned when no session has the given ID.
	ErrNotFound = errors.New("session not found")
	// ErrUnknownItem is returned when an answer names an item not on the form.
	ErrUnknownItem = errors.New("unknown item")
	// ErrBadResponse is returned when an answer does not fit the item, for
	// example a response the item does not declare.
	ErrBadResponse = errors.New("bad response")
	// ErrAlreadyDone is returned when a submitted session is answered or
	// submitted again.
	ErrAlreadyDone = errors.New("session already submitted")
)

// attempt is a session together with the candidate's answers, which are kept
// out of Session so they are never returned to a caller.
type attempt struct {
	session   Session
	responses map[string]qti.Responses // item identifier -> answers
}

// Store holds every session in memory. It is safe for concurrent use.
type Store struct {
	mu       sync.RWMutex
	items    []*qti.Item
	byID     map[string]*qti.Item
	sessions map[string]*attempt
	now      func() time.Time
}

// NewStore returns an empty store for the form made of items, in order. The
// items are shared with the store and must not be modified afterwards.
func NewStore(items []*qti.Item) *Store {
	byID := make(map[string]*qti.Item, len(items))
	for _, it := range items {
		byID[it.Identifier] = it
	}
	return &Store{
		items:    items,
		byID:     byID,
		sessions: make(map[string]*attempt),
		now:      time.Now,
	}
}

// newID returns a random session ID that cannot be guessed.
func newID() string {
	return rand.Text()
}

// clone returns a copy of s that shares no memory with it, so a caller can
// change the copy without changing the stored session.
func (s Session) clone() Session {
	s.Results = slices.Clone(s.Results)
	return s
}

// Start begins a new in-progress session for the candidate. It returns an
// error if candidateID is empty.
func (s *Store) Start(candidateID string) (Session, error) {
	if candidateID == "" {
		return Session{}, errors.New("session: empty candidate ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	a := &attempt{
		session: Session{
			ID:          newID(),
			CandidateID: candidateID,
			Status:      StatusInProgress,
			StartedAt:   s.now(),
		},
		responses: make(map[string]qti.Responses),
	}
	s.sessions[a.session.ID] = a
	return a.session.clone(), nil
}

// Get returns the session with the given ID, or ErrNotFound.
func (s *Store) Get(id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return a.session.clone(), nil
}
