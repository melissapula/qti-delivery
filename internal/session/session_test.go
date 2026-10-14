package session

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
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
				`"results":[{"itemIdentifier":"capital-mn","score":1,"maxScore":1,"correct":true}],` +
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

// answer is one call to Answer, for tables that replay several.
type answer struct {
	itemID    string
	responses qti.Responses
}

// startAndAnswer starts a session and records answers in order, failing the
// test on any error.
func startAndAnswer(t *testing.T, s *Store, answers []answer) Session {
	t.Helper()
	started, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, a := range answers {
		if err := s.Answer(started.ID, a.itemID, a.responses); err != nil {
			t.Fatalf("Answer(%s, %v): %v", a.itemID, a.responses, err)
		}
	}
	return started
}

func TestSubmit(t *testing.T) {
	allCorrect := []answer{
		{"capital-mn", qti.Responses{"RESPONSE": {"B"}}},
		{"primes", qti.Responses{"RESPONSE": {"A", "C", "D"}}},
		{"lake-source", qti.Responses{"RESPONSE": {"itasca"}}},
	}

	tests := []struct {
		name      string
		answers   []answer
		want      []qti.Result
		wantScore float64
	}{
		{
			name:    "all three answered correctly",
			answers: allCorrect,
			want: []qti.Result{
				{ItemIdentifier: "capital-mn", Score: 1, MaxScore: 1, Correct: true},
				{ItemIdentifier: "primes", Score: 3, MaxScore: 3, Correct: true},
				{ItemIdentifier: "lake-source", Score: 1, MaxScore: 1, Correct: true},
			},
			wantScore: 5,
		},
		{
			name:    "only capital-mn answered",
			answers: allCorrect[:1],
			want: []qti.Result{
				{ItemIdentifier: "capital-mn", Score: 1, MaxScore: 1, Correct: true},
				{ItemIdentifier: "primes", Score: 0, MaxScore: 3},
				{ItemIdentifier: "lake-source", Score: 0, MaxScore: 1},
			},
			wantScore: 1,
		},
		{
			name: "nothing answered",
			want: []qti.Result{
				{ItemIdentifier: "capital-mn", Score: 0, MaxScore: 1},
				{ItemIdentifier: "primes", Score: 0, MaxScore: 3},
				{ItemIdentifier: "lake-source", Score: 0, MaxScore: 1},
			},
		},
		{
			name: "a later answer replaces an earlier one",
			answers: []answer{
				{"capital-mn", qti.Responses{"RESPONSE": {"A"}}},
				{"capital-mn", qti.Responses{"RESPONSE": {"B"}}},
			},
			want: []qti.Result{
				{ItemIdentifier: "capital-mn", Score: 1, MaxScore: 1, Correct: true},
				{ItemIdentifier: "primes", Score: 0, MaxScore: 3},
				{ItemIdentifier: "lake-source", Score: 0, MaxScore: 1},
			},
			wantScore: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			started := startAndAnswer(t, s, tt.answers)

			got, err := s.Submit(started.ID)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if got.Status != StatusSubmitted {
				t.Errorf("Status = %q, want %q", got.Status, StatusSubmitted)
			}
			if !got.SubmittedAt.Equal(testNow) {
				t.Errorf("SubmittedAt = %v, want %v", got.SubmittedAt, testNow)
			}
			if !reflect.DeepEqual(got.Results, tt.want) {
				t.Errorf("Results =\n%+v\nwant\n%+v", got.Results, tt.want)
			}
			if got.Score != tt.wantScore || got.MaxScore != 5 {
				t.Errorf("total = %v of %v, want %v of 5", got.Score, got.MaxScore, tt.wantScore)
			}

			stored, err := s.Get(started.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if !reflect.DeepEqual(stored, got) {
				t.Errorf("Get after Submit = %+v, want %+v", stored, got)
			}
		})
	}
}

func TestSubmitErrors(t *testing.T) {
	t.Run("unknown session", func(t *testing.T) {
		s := newTestStore(t)
		if _, err := s.Submit("no-such-session"); !errors.Is(err, ErrNotFound) {
			t.Errorf("Submit error = %v, want ErrNotFound", err)
		}
	})

	t.Run("submit twice", func(t *testing.T) {
		s := newTestStore(t)
		started := startAndAnswer(t, s, nil)
		if _, err := s.Submit(started.ID); err != nil {
			t.Fatalf("first Submit: %v", err)
		}
		if _, err := s.Submit(started.ID); !errors.Is(err, ErrAlreadyDone) {
			t.Errorf("second Submit error = %v, want ErrAlreadyDone", err)
		}
	})

	t.Run("answer after submit", func(t *testing.T) {
		s := newTestStore(t)
		started := startAndAnswer(t, s, nil)
		if _, err := s.Submit(started.ID); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		err := s.Answer(started.ID, "capital-mn", qti.Responses{"RESPONSE": {"B"}})
		if !errors.Is(err, ErrAlreadyDone) {
			t.Errorf("Answer error = %v, want ErrAlreadyDone", err)
		}
	})
}

// TestSubmitScoreErrorLeavesSessionOpen uses an item Score always rejects, so
// Submit fails part way, and checks the session is not half submitted.
func TestSubmitScoreErrorLeavesSessionOpen(t *testing.T) {
	unsupported := &qti.Item{
		Identifier: "odd-template",
		ResponseDeclarations: []qti.ResponseDeclaration{
			{Identifier: "RESPONSE", Cardinality: "single", BaseType: "identifier"},
		},
		ResponseProcessing: qti.ResponseProcessing{Template: "map_response_point"},
	}
	s := NewStore([]*qti.Item{unsupported})
	s.now = func() time.Time { return testNow }
	started, err := s.Start("cand-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := s.Submit(started.ID); err == nil {
		t.Fatal("Submit returned nil error, want the Score error")
	}
	got, err := s.Get(started.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, started) {
		t.Errorf("after a failed Submit, session = %+v, want it unchanged %+v", got, started)
	}
}

// TestRejectedAnswerNotStored checks that an answer Answer rejects does not
// replace the earlier, valid one.
func TestRejectedAnswerNotStored(t *testing.T) {
	s := newTestStore(t)
	started := startAndAnswer(t, s, []answer{{"capital-mn", qti.Responses{"RESPONSE": {"B"}}}})

	err := s.Answer(started.ID, "capital-mn", qti.Responses{"RESPONSE": {"A", "C"}})
	if !errors.Is(err, ErrBadResponse) {
		t.Fatalf("Answer error = %v, want ErrBadResponse", err)
	}
	got, err := s.Submit(started.ID)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !got.Results[0].Correct {
		t.Errorf("capital-mn result = %+v, want the earlier correct answer kept", got.Results[0])
	}
}

func TestSubmitStoresCopy(t *testing.T) {
	s := newTestStore(t)
	started := startAndAnswer(t, s, nil)
	r := qti.Responses{"RESPONSE": {"B"}}
	if err := s.Answer(started.ID, "capital-mn", r); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	r["RESPONSE"][0] = "A" // the caller reuses its slice after answering

	got, err := s.Submit(started.ID)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !got.Results[0].Correct {
		t.Errorf("capital-mn result = %+v, want it scored on the answer as given", got.Results[0])
	}
}

func TestGetReturnsCopy(t *testing.T) {
	tests := []struct {
		name string
		// returned gives the session whose Results the test changes.
		returned func(s *Store, id string) (Session, error)
	}{
		{"from Submit", func(s *Store, id string) (Session, error) { return s.Submit(id) }},
		{"from Get", func(s *Store, id string) (Session, error) {
			if _, err := s.Submit(id); err != nil {
				return Session{}, err
			}
			return s.Get(id)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			started := startAndAnswer(t, s, []answer{{"capital-mn", qti.Responses{"RESPONSE": {"B"}}}})
			got, err := tt.returned(s, started.ID)
			if err != nil {
				t.Fatalf("setup: %v", err)
			}

			got.Results[0].Score = 99

			again, err := s.Get(started.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if again.Results[0].Score != 1 {
				t.Errorf("stored Results[0].Score = %v after changing a returned copy, want 1", again.Results[0].Score)
			}
		})
	}
}

func TestItems(t *testing.T) {
	s := newTestStore(t)

	first := s.Items()
	var ids []string
	for _, it := range first {
		ids = append(ids, it.Identifier)
	}
	want := []string{"capital-mn", "primes", "lake-source"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("Items identifiers = %v, want %v", ids, want)
	}

	first[0], first[2] = first[2], first[0]

	if got := s.Items()[0].Identifier; got != "capital-mn" {
		t.Errorf("after reordering a returned slice, Items()[0] = %q, want %q", got, "capital-mn")
	}
}

// TestConcurrentStart starts many sessions at once. Run it with -race: a
// missing lock may still pass a plain run.
func TestConcurrentStart(t *testing.T) {
	s := newTestStore(t)
	const n = 100

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex // guards ids, which the goroutines share
		ids = make(map[string]bool)
	)
	for range n {
		wg.Go(func() {
			got, err := s.Start("cand-1")
			if err != nil {
				t.Errorf("Start: %v", err)
				return
			}
			mu.Lock()
			ids[got.ID] = true
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(ids) != n {
		t.Errorf("%d distinct IDs from %d starts, want %d", len(ids), n, n)
	}
	for id := range ids {
		if _, err := s.Get(id); err != nil {
			t.Errorf("Get(%s): %v", id, err)
		}
	}
}

// TestConcurrentAnswerGet answers and reads one session from many goroutines,
// then submits it while late answers are still arriving. Its job is to give
// -race reads and writes on the same state.
func TestConcurrentAnswerGet(t *testing.T) {
	s := newTestStore(t)
	started := startAndAnswer(t, s, nil)
	const n = 50

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if err := s.Answer(started.ID, "capital-mn", qti.Responses{"RESPONSE": {"B"}}); err != nil {
				t.Errorf("Answer: %v", err)
			}
		})
		wg.Go(func() {
			if _, err := s.Get(started.ID); err != nil {
				t.Errorf("Get: %v", err)
			}
		})
	}
	wg.Wait()

	// Each late answer lands either before the submit or after it, when it
	// must be refused.
	for range n {
		wg.Go(func() {
			err := s.Answer(started.ID, "primes", qti.Responses{"RESPONSE": {"A", "C", "D"}})
			if err != nil && !errors.Is(err, ErrAlreadyDone) {
				t.Errorf("late Answer error = %v, want nil or ErrAlreadyDone", err)
			}
		})
	}
	wg.Go(func() {
		if _, err := s.Submit(started.ID); err != nil {
			t.Errorf("Submit: %v", err)
		}
	})
	wg.Wait()

	got, err := s.Get(started.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusSubmitted {
		t.Errorf("Status = %q, want %q", got.Status, StatusSubmitted)
	}
}
