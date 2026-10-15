package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/melissapula/qti-delivery/internal/qti"
	"github.com/melissapula/qti-delivery/internal/session"
)

// itemsDir is the sample test form, relative to this package's directory.
const itemsDir = "../../testdata/items"

// baseURL is any address: the test server's client sends every request to
// the test server.
const baseURL = "http://qti.test"

// newTestStore returns an empty session store over the sample form.
func newTestStore(t *testing.T) *session.Store {
	t.Helper()
	items, err := qti.LoadDir(itemsDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	return session.NewStore(items)
}

// newTestAPI serves the API over the sample form on an in-memory test server
// and returns a client for it. The server shuts down when the test ends.
func newTestAPI(t *testing.T) *http.Client {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewTestServer(t, New(newTestStore(t), log)).Client()
}

// do sends a request with an optional body and returns the response with its
// body read.
func do(t *testing.T, c *http.Client, method, path, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(b)
}

// checkJSON checks that resp has the wanted status and a JSON content type.
func checkJSON(t *testing.T, resp *http.Response, wantStatus int) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d", resp.StatusCode, wantStatus)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHealthz(t *testing.T) {
	resp, body := do(t, newTestAPI(t), http.MethodGet, "/healthz", "")

	checkJSON(t, resp, http.StatusOK)
	if got, want := strings.TrimSpace(body), `{"status":"ok"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

func TestListItems(t *testing.T) {
	resp, body := do(t, newTestAPI(t), http.MethodGet, "/api/items", "")

	checkJSON(t, resp, http.StatusOK)
	want := `[` +
		`{"index":0,"identifier":"capital-mn","title":"State Capital","maxScore":1},` +
		`{"index":1,"identifier":"primes","title":"Prime Numbers","maxScore":3},` +
		`{"index":2,"identifier":"lake-source","title":"Headwaters","maxScore":1}` +
		`]`
	if got := strings.TrimSpace(body); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

// checkNoAnswers fails if body holds an answer from the sample form or the
// name of a field that carries answers. It ignores case, so it also catches
// an item marshalled with its Go field names (CorrectResponse, Mapping).
func checkNoAnswers(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, secret := range []string{"itasca", "correctresponse", "mapping"} {
		if strings.Contains(lower, secret) {
			t.Errorf("body contains %q:\n%s", secret, body)
		}
	}
}

func TestListItemsDoesNotLeak(t *testing.T) {
	_, body := do(t, newTestAPI(t), http.MethodGet, "/api/items", "")
	checkNoAnswers(t, body)
}

// TestUnmatchedRoutes pins that ServeMux answers paths and methods the API
// does not serve, in plain text, so changing that is a deliberate decision.
func TestUnmatchedRoutes(t *testing.T) {
	c := newTestAPI(t)

	t.Run("unknown path", func(t *testing.T) {
		resp, _ := do(t, c, http.MethodGet, "/nope", "")
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		resp, _ := do(t, c, http.MethodDelete, "/healthz", "")
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", resp.StatusCode)
		}
		if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
			t.Errorf("Allow = %q, want it to list GET", allow)
		}
	})
}

// decodeAs decodes a JSON response body into a T.
func decodeAs[T any](t *testing.T, body string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return v
}

// sessionBody is the part of a session response the tests check.
type sessionBody struct {
	ID          string       `json:"id"`
	CandidateID string       `json:"candidateId"`
	Status      string       `json:"status"`
	Results     []qti.Result `json:"results"`
	Score       float64      `json:"score"`
	MaxScore    float64      `json:"maxScore"`
}

// itemBody is an item response.
type itemBody struct {
	Index int      `json:"index"`
	Total int      `json:"total"`
	Item  qti.View `json:"item"`
}

// startSession starts a session for candidate c-123 and returns its ID.
func startSession(t *testing.T, c *http.Client) string {
	t.Helper()
	resp, body := do(t, c, http.MethodPost, "/api/sessions", `{"candidateId":"c-123"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start: status = %d, want 201; body %s", resp.StatusCode, body)
	}
	return decodeAs[sessionBody](t, body).ID
}

// submitSession submits the session.
func submitSession(t *testing.T, c *http.Client, id string) {
	t.Helper()
	resp, body := do(t, c, http.MethodPost, "/api/sessions/"+id+"/submit", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit: status = %d, want 200; body %s", resp.StatusCode, body)
	}
}

// TestEndToEnd runs the spec's example session: start, fetch an item, answer
// every item correctly, submit and check the score.
func TestEndToEnd(t *testing.T) {
	c := newTestAPI(t)

	resp, body := do(t, c, http.MethodPost, "/api/sessions", `{"candidateId":"c-123"}`)
	checkJSON(t, resp, http.StatusCreated)
	started := decodeAs[sessionBody](t, body)
	if started.ID == "" || started.CandidateID != "c-123" || started.Status != "in_progress" {
		t.Fatalf("started session = %+v, want an ID, candidate c-123, in_progress", started)
	}
	base := "/api/sessions/" + started.ID

	resp, body = do(t, c, http.MethodGet, base+"/items/0", "")
	checkJSON(t, resp, http.StatusOK)
	if !strings.HasPrefix(body, `{"index":0,"total":3,"item":{`) {
		t.Errorf("item body = %s, want it to start with index, total and item", body)
	}
	if item := decodeAs[itemBody](t, body); item.Item.Identifier != "capital-mn" {
		t.Errorf("item 0 identifier = %q, want capital-mn", item.Item.Identifier)
	}

	answers := []struct{ itemID, body string }{
		{"capital-mn", `{"RESPONSE":["B"]}`},
		{"primes", `{"RESPONSE":["A","C","D"]}`},
		{"lake-source", `{"RESPONSE":["itasca"]}`},
	}
	for _, a := range answers {
		resp, body := do(t, c, http.MethodPut, base+"/responses/"+a.itemID, a.body)
		if resp.StatusCode != http.StatusNoContent || body != "" {
			t.Errorf("answer %s: status %d body %q, want 204 and no body", a.itemID, resp.StatusCode, body)
		}
	}

	resp, body = do(t, c, http.MethodPost, base+"/submit", "")
	checkJSON(t, resp, http.StatusOK)
	submitted := decodeAs[sessionBody](t, body)
	if submitted.Status != "submitted" || submitted.Score != 5 || submitted.MaxScore != 5 {
		t.Errorf("submitted = %s, score %v of %v; want submitted, 5 of 5",
			submitted.Status, submitted.Score, submitted.MaxScore)
	}
	wantResults := []qti.Result{
		{ItemIdentifier: "capital-mn", Score: 1, MaxScore: 1, Correct: true},
		{ItemIdentifier: "primes", Score: 3, MaxScore: 3, Correct: true},
		{ItemIdentifier: "lake-source", Score: 1, MaxScore: 1, Correct: true},
	}
	if !slices.Equal(submitted.Results, wantResults) {
		t.Errorf("results = %+v, want %+v", submitted.Results, wantResults)
	}

	resp, body = do(t, c, http.MethodGet, base, "")
	checkJSON(t, resp, http.StatusOK)
	if got := decodeAs[sessionBody](t, body); got.Status != "submitted" || got.Score != 5 {
		t.Errorf("session after submit = %s, score %v; want submitted, 5", got.Status, got.Score)
	}
}

func TestErrorStatuses(t *testing.T) {
	c := newTestAPI(t)
	open := "/api/sessions/" + startSession(t, c)
	doneID := startSession(t, c)
	submitSession(t, c, doneID)
	done := "/api/sessions/" + doneID
	missing := "/api/sessions/no-such-session"

	big := strings.Repeat("a", maxBodyBytes)

	tests := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"get unknown session", http.MethodGet, missing, "", http.StatusNotFound},
		{"item of unknown session", http.MethodGet, missing + "/items/0", "", http.StatusNotFound},
		{"answer unknown session", http.MethodPut, missing + "/responses/capital-mn", `{"RESPONSE":["B"]}`, http.StatusNotFound},
		{"submit unknown session", http.MethodPost, missing + "/submit", "", http.StatusNotFound},
		{"answer unknown item", http.MethodPut, open + "/responses/no-such-item", `{"RESPONSE":["B"]}`, http.StatusNotFound},
		{"item index past the end", http.MethodGet, open + "/items/3", "", http.StatusNotFound},
		{"negative item index", http.MethodGet, open + "/items/-1", "", http.StatusNotFound},

		{"answer after submit", http.MethodPut, done + "/responses/capital-mn", `{"RESPONSE":["B"]}`, http.StatusConflict},
		{"submit twice", http.MethodPost, done + "/submit", "", http.StatusConflict},

		{"two values for a single answer", http.MethodPut, open + "/responses/capital-mn", `{"RESPONSE":["A","B"]}`, http.StatusUnprocessableEntity},
		{"undeclared response", http.MethodPut, open + "/responses/capital-mn", `{"RESPONS":["B"]}`, http.StatusUnprocessableEntity},
		{
			// Answer checks the item before the session, so the bad
			// response is reported, not the missing session.
			"bad response to unknown session", http.MethodPut, missing + "/responses/capital-mn",
			`{"RESPONSE":["A","B"]}`, http.StatusUnprocessableEntity,
		},

		{"start with malformed JSON", http.MethodPost, "/api/sessions", `{"candidateId":`, http.StatusBadRequest},
		{"start with unknown field", http.MethodPost, "/api/sessions", `{"candidateId":"c-1","extra":1}`, http.StatusBadRequest},
		{"start with trailing data", http.MethodPost, "/api/sessions", `{"candidateId":"c-1"}{}`, http.StatusBadRequest},
		{"start with empty body", http.MethodPost, "/api/sessions", "", http.StatusBadRequest},
		{"start with empty candidateId", http.MethodPost, "/api/sessions", `{"candidateId":""}`, http.StatusBadRequest},
		{"start with no candidateId", http.MethodPost, "/api/sessions", `{}`, http.StatusBadRequest},
		{"answer with wrong JSON type", http.MethodPut, open + "/responses/capital-mn", `{"RESPONSE":"B"}`, http.StatusBadRequest},
		{"answer with empty body", http.MethodPut, open + "/responses/capital-mn", "", http.StatusBadRequest},
		{"non-numeric item index", http.MethodGet, open + "/items/x", "", http.StatusBadRequest},

		{"start body over 1 MiB", http.MethodPost, "/api/sessions", `{"candidateId":"` + big + `"}`, http.StatusRequestEntityTooLarge},
		{"answer body over 1 MiB", http.MethodPut, open + "/responses/capital-mn", `{"RESPONSE":["` + big + `"]}`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := do(t, c, tt.method, tt.path, tt.body)
			checkJSON(t, resp, tt.want)
			if msg := decodeAs[errorBody](t, body).Error; msg == "" {
				t.Errorf("body %s has no error message", body)
			}
		})
	}
}

// TestGetItemAfterSubmit checks that a submitted session can still read the
// items: a view holds no answers, so there is nothing to protect.
func TestGetItemAfterSubmit(t *testing.T) {
	c := newTestAPI(t)
	id := startSession(t, c)
	submitSession(t, c, id)

	resp, _ := do(t, c, http.MethodGet, "/api/sessions/"+id+"/items/0", "")
	checkJSON(t, resp, http.StatusOK)
}

// TestItemsDoNotLeak guards the item handler against returning an item
// instead of its view. TestViewDoesNotLeakAnswers in package qti is the
// real proof that a view holds no answers.
func TestItemsDoNotLeak(t *testing.T) {
	c := newTestAPI(t)
	base := "/api/sessions/" + startSession(t, c)

	for i := range 3 {
		resp, body := do(t, c, http.MethodGet, base+"/items/"+strconv.Itoa(i), "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("item %d: status = %d, want 200", i, resp.StatusCode)
		}
		checkNoAnswers(t, body)
	}
}
