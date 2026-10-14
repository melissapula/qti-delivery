package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// newTestAPI serves the API over the sample form on an in-memory test server
// and returns a client for it. The server shuts down when the test ends.
func newTestAPI(t *testing.T) *http.Client {
	t.Helper()
	items, err := qti.LoadDir(itemsDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewTestServer(t, New(session.NewStore(items), log)).Client()
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
