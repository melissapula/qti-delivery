package session

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/melissapula/qti-delivery/internal/qti"
)

// itemsDir is the sample test form, relative to this package's directory.
const itemsDir = "../../testdata/items"

// testNow is the fixed clock every test store uses, so times can be compared
// exactly.
var testNow = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

// newTestStore returns a store over the sample form with its clock fixed at
// testNow.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	items, err := qti.LoadDir(itemsDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	s := NewStore(items)
	s.now = func() time.Time { return testNow }
	return s
}

// TestSessionJSON pins the JSON names the HTTP API will use, so renaming a
// field is a deliberate test change rather than a silent client break.
func TestSessionJSON(t *testing.T) {
	tests := []struct {
		name    string
		session Session
		want    string
	}{
		{
			name: "in progress omits submittedAt and results",
			session: Session{
				ID:          "abc",
				CandidateID: "cand-1",
				Status:      StatusInProgress,
				StartedAt:   testNow,
			},
			want: `{"id":"abc","candidateId":"cand-1","status":"in_progress","startedAt":"2026-10-14T09:00:00Z",` +
				`"score":0,"maxScore":0}`,
		},
		{
			name: "submitted includes submittedAt, results and totals",
			session: Session{
				ID:          "abc",
				CandidateID: "cand-1",
				Status:      StatusSubmitted,
				StartedAt:   testNow,
				SubmittedAt: testNow.Add(30 * time.Minute),
				Results: []qti.Result{
					{ItemIdentifier: "capital-mn", Score: 1, MaxScore: 1, Correct: true},
				},
				Score:    1,
				MaxScore: 1,
			},
			want: `{"id":"abc","candidateId":"cand-1","status":"submitted","startedAt":"2026-10-14T09:00:00Z",` +
				`"submittedAt":"2026-10-14T09:30:00Z",` +
				`"results":[{"ItemIdentifier":"capital-mn","Score":1,"MaxScore":1,"Correct":true}],` +
				`"score":1,"maxScore":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.session)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("JSON =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestStart(t *testing.T) {
	s := newTestStore(t)

	got, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}
	if got.CandidateID != "cand-1" {
		t.Errorf("CandidateID = %q, want %q", got.CandidateID, "cand-1")
	}
	if got.Status != StatusInProgress {
		t.Errorf("Status = %q, want %q", got.Status, StatusInProgress)
	}
	if !got.StartedAt.Equal(testNow) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, testNow)
	}
	if !got.SubmittedAt.IsZero() {
		t.Errorf("SubmittedAt = %v, want zero", got.SubmittedAt)
	}
}

func TestStartEmptyCandidateID(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.Start(""); err == nil {
		t.Error("Start(\"\") returned nil error, want an error")
	}
}

func TestStartIDsDiffer(t *testing.T) {
	s := newTestStore(t)

	first, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	second, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if first.ID == second.ID {
		t.Errorf("two starts gave the same ID %q", first.ID)
	}
}

func TestGet(t *testing.T) {
	s := newTestStore(t)
	started, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	got, err := s.Get(started.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, started) {
		t.Errorf("Get = %+v, want %+v", got, started)
	}
}

func TestGetUnknownID(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Get("no-such-session")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get error = %v, want ErrNotFound", err)
	}
}

func TestCloneResponses(t *testing.T) {
	// Each case builds its input with a function, so the test can call it
	// twice: once for the input to clone and mutate, once for the value the
	// clone must still equal.
	tests := []struct {
		name string
		in   func() qti.Responses
	}{
		{"nil", func() qti.Responses { return nil }},
		{"empty", func() qti.Responses { return qti.Responses{} }},
		{"key with no values", func() qti.Responses { return qti.Responses{"RESPONSE": {}} }},
		{"one key", func() qti.Responses { return qti.Responses{"RESPONSE": {"B"}} }},
		{"several keys", func() qti.Responses {
			return qti.Responses{"RESPONSE": {"A", "C", "D"}, "RESPONSE_2": {"itasca"}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in()
			got := cloneResponses(in)
			want := tt.in()
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("cloneResponses = %#v, want %#v", got, want)
			}

			if in == nil {
				return // nothing to mutate
			}
			for _, values := range in {
				if len(values) > 0 {
					values[0] = "changed"
				}
			}
			in["ADDED"] = []string{"X"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("after changing the input, clone = %#v, want %#v", got, want)
			}
		})
	}
}

func TestAnswerErrors(t *testing.T) {
	s := newTestStore(t)
	started, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	tests := []struct {
		name       string
		sessionID  string
		itemID     string
		responses  qti.Responses
		wantErr    error
		wantDetail string // part of Score's message that must survive wrapping
	}{
		{
			name:      "unknown session",
			sessionID: "no-such-session",
			itemID:    "capital-mn",
			responses: qti.Responses{"RESPONSE": {"B"}},
			wantErr:   ErrNotFound,
		},
		{
			name:      "unknown item",
			sessionID: started.ID,
			itemID:    "no-such-item",
			responses: qti.Responses{"RESPONSE": {"B"}},
			wantErr:   ErrUnknownItem,
		},
		{
			// Item checks come before the session lookup.
			name:      "unknown item on unknown session",
			sessionID: "no-such-session",
			itemID:    "no-such-item",
			responses: qti.Responses{"RESPONSE": {"B"}},
			wantErr:   ErrUnknownItem,
		},
		{
			name:       "undeclared response",
			sessionID:  started.ID,
			itemID:     "capital-mn",
			responses:  qti.Responses{"RESPONS": {"B"}},
			wantErr:    ErrBadResponse,
			wantDetail: `unknown response "RESPONS"`,
		},
		{
			name:       "two values for a single-answer choice",
			sessionID:  started.ID,
			itemID:     "capital-mn",
			responses:  qti.Responses{"RESPONSE": {"B", "A"}},
			wantErr:    ErrBadResponse,
			wantDetail: `response "RESPONSE" takes one value, got 2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Answer(tt.sessionID, tt.itemID, tt.responses)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Answer error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Errorf("Answer error = %q, want it to contain %q", err, tt.wantDetail)
			}
		})
	}
}

func TestAnswerAccepts(t *testing.T) {
	s := newTestStore(t)
	started, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	tests := []struct {
		itemID    string
		responses qti.Responses
	}{
		{"capital-mn", qti.Responses{"RESPONSE": {"B"}}},
		{"primes", qti.Responses{"RESPONSE": {"A", "C", "D"}}},
		{"lake-source", qti.Responses{"RESPONSE": {"itasca"}}},
		{"capital-mn", qti.Responses{}}, // blank answer, which Score accepts
	}

	for _, tt := range tests {
		if err := s.Answer(started.ID, tt.itemID, tt.responses); err != nil {
			t.Errorf("Answer(%s, %v) = %v, want nil", tt.itemID, tt.responses, err)
		}
	}
}
