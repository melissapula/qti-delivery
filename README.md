# QTI Delivery Engine

![CI](https://github.com/melissapula/qti-delivery/actions/workflows/ci.yml/badge.svg)

A small assessment delivery service in Go, using only the standard library. It loads test
questions written in the IMS QTI 3.0 standard, delivers them to a candidate one at a time without
the answers, records responses, and scores the attempt with the `match_correct` and `map_response`
templates.

## Why this project, and why QTI

Edtech has been an interest of mine ever since I met my wife, who was a teacher for 20 years. I
also wanted real experience in Go, because many of the job descriptions I read ask for it. This
project brings the two together: instead of following a tutorial, I built a small but realistic
piece of an assessment platform.

I chose QTI because it is the standard testing platforms actually use to exchange questions, so
working from it meant handling a real specification rather than a format I made up.

## Scope

In scope:

- QTI 3.0 items loaded from a folder of XML files; the folder is one linear test form.
- `qti-choice-interaction` (single and multiple answer) and `qti-text-entry-interaction`.
- Scoring with `match_correct` and `map_response`, including `lower-bound`, `upper-bound`,
  `default-value` and `case-sensitive` on map entries.
- A JSON HTTP API to start a session, fetch items without answers, save responses, and submit for
  a score.
- An in-memory session store that is safe for concurrent requests.

Out of scope for now: persistence (sessions are lost on restart), authentication, other
interaction types, custom response processing, test-level structure (`qti-assessment-test`), and a
frontend.

## Running it

You need Go 1.27.2 or newer.

```bash
go run ./cmd/server
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8080` | address to listen on |
| `-items` | `testdata/items` | folder of QTI item XML files that make up the test form |

Files load in name order, which is why the samples are numbered. Ctrl+C shuts the server down
gracefully: it stops accepting connections and waits up to 10 seconds for requests in progress.

The Makefile has shortcuts for the same checks CI runs:

| Command | What it does |
|---|---|
| `make test` | `go test ./...` |
| `make race` | the tests under the race detector (needs a C compiler; on Windows, install one such as MinGW-w64) |
| `make lint` | fails if any file needs `gofmt`, then runs `go vet ./...` |
| `make run` | starts the server with the sample items |

## API

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/healthz` | | `{"status":"ok"}` |
| GET | `/api/items` | | form outline: index, identifier, title, maxScore |
| POST | `/api/sessions` | `{"candidateId":"c-123"}` | 201 and the session |
| GET | `/api/sessions/{id}` | | the session |
| GET | `/api/sessions/{id}/items/{index}` | | `{index, total, item}`, where `item` is the candidate's view |
| PUT | `/api/sessions/{id}/responses/{itemId}` | `{"RESPONSE":["B"]}` | 204 |
| POST | `/api/sessions/{id}/submit` | | the session with results |

Errors are JSON (`{"error":"..."}`) with these statuses: 400 for a malformed body, a missing
field or a non-numeric item index, 404 for an unknown session or item, 409 if the session is already submitted, 413 for a body
over 1 MiB, 422 for a response the item rejects, and 500 for anything else, which is also logged.
Unknown JSON fields are rejected. Paths the API does not serve, and wrong methods on paths it
does, get Go's plain-text 404 and 405 responses.

## Example session

Start the server with `make run`, then in another terminal (responses are from a real run; the
session ID is shortened to `<SESSION>`):

```bash
curl -s -X POST localhost:8080/api/sessions -d '{"candidateId":"c-123"}'
# {"id":"<SESSION>","candidateId":"c-123","status":"in_progress","startedAt":"2026-10-14T20:25:15.3342123-05:00","score":0,"maxScore":0}

curl -s localhost:8080/api/sessions/<SESSION>/items/0
# {"index":0,"total":3,"item":{"identifier":"capital-mn","title":"State Capital","interactions":[{"type":"choice","responseIdentifier":"RESPONSE","prompt":"What is the capital of Minnesota?","maxChoices":1,"choices":[{"identifier":"A","text":"Minneapolis"},{"identifier":"B","text":"Saint Paul"},{"identifier":"C","text":"Duluth"},{"identifier":"D","text":"Bemidji"}]}]}}

curl -s localhost:8080/api/sessions/<SESSION>/items/2
# {"index":2,"total":3,"item":{"identifier":"lake-source","title":"Headwaters","paragraphs":["The Mississippi River begins at Lake {{RESPONSE}} in northern Minnesota."],"interactions":[{"type":"textEntry","responseIdentifier":"RESPONSE","expectedLength":15}]}}

curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/capital-mn -d '{"RESPONSE":["B"]}'
curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/primes -d '{"RESPONSE":["A","C","D"]}'
curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/lake-source -d '{"RESPONSE":["itasca"]}'
# each returns 204 with no body

curl -s -X POST localhost:8080/api/sessions/<SESSION>/submit
```

The submit response, formatted for reading:

```json
{
  "id": "<SESSION>",
  "candidateId": "c-123",
  "status": "submitted",
  "startedAt": "2026-10-14T20:25:15.3342123-05:00",
  "submittedAt": "2026-10-14T20:25:16.2211644-05:00",
  "results": [
    {"itemIdentifier": "capital-mn", "score": 1, "maxScore": 1, "correct": true},
    {"itemIdentifier": "primes", "score": 3, "maxScore": 3, "correct": true},
    {"itemIdentifier": "lake-source", "score": 1, "maxScore": 1, "correct": true}
  ],
  "score": 5,
  "maxScore": 5
}
```

Submitting again returns 409 with `{"error":"session already submitted"}`.

The item views carry no correct answers or score mappings; the text-entry item marks where the
answer box goes with `{{RESPONSE}}`.

In Windows PowerShell 5.1, `curl.exe` strips the double quotes inside JSON arguments, so use
`Invoke-RestMethod` instead (it needs the `http://` prefix):

```powershell
$base = 'http://localhost:8080'
$s = Invoke-RestMethod -Method Post "$base/api/sessions" -Body '{"candidateId":"c-123"}'
Invoke-RestMethod "$base/api/sessions/$($s.id)/items/0"
Invoke-RestMethod -Method Put "$base/api/sessions/$($s.id)/responses/capital-mn" -Body '{"RESPONSE":["B"]}'
Invoke-RestMethod -Method Put "$base/api/sessions/$($s.id)/responses/primes" -Body '{"RESPONSE":["A","C","D"]}'
Invoke-RestMethod -Method Put "$base/api/sessions/$($s.id)/responses/lake-source" -Body '{"RESPONSE":["itasca"]}'
$r = Invoke-RestMethod -Method Post "$base/api/sessions/$($s.id)/submit"
"score $($r.score) of $($r.maxScore)"   # score 5 of 5
```

## Design decisions

- **Standard library only.** `encoding/xml`, `encoding/json`, `net/http`, `log/slog`, `sync`
  and `crypto/rand` cover everything here, and part of the point was to learn what Go provides.
- **`internal/` packages.** `qti` (parsing, scoring, candidate view), `session` (attempts) and
  `api` (HTTP) cannot be imported from outside this module, so they are free to change.
- **Answers cannot leak by construction.** The parsed item holds the correct responses and score
  mappings; the candidate sees a separate `View` type that has no fields for them, so there is
  nothing to accidentally serialize. Tests marshal every candidate-facing response and check that
  the correct answers do not appear anywhere in it. In high-stakes testing an answer key in an API
  response is an incident, not a bug.
- **Concurrency-safe sessions.** The store guards its state with a `sync.RWMutex` and hands out
  copies, so callers cannot change a session behind its back. Tests start and answer sessions from
  many goroutines and run under the race detector, locally and in CI.
- **Testable time and IDs.** The store's clock is a function field that tests replace; session
  IDs come from `crypto/rand`.
- **No framework.** Routing uses Go's `http.ServeMux` patterns with methods and path values, and
  request logging is a small middleware that wraps the mux.
- **Defensive HTTP.** Request bodies are capped at 1 MiB, unknown JSON fields are rejected, and
  the server sets a header read timeout so a slow client cannot hold a connection open.
- **Clean startup and shutdown.** The server listens before it starts serving, so a busy port is
  reported immediately, and Ctrl+C triggers a graceful shutdown with a deadline.

## What I'd add next

- Persist sessions in SQLite or Postgres behind an interface, so the store is swappable.
- Support `qti-assessment-test` with sections and time limits.
- Add `qti-order-interaction` and `qti-match-interaction`.
- A small Angular frontend that consumes the API.
- A Dockerfile and a deployment.
- Accessibility notes for the candidate view; WCAG matters a lot in K-12 testing.

Known limitations to fix:

- Scoring accepts choice values the item does not offer, and ignores `max-choices`.
- Item validation accepts a `map_response` item with no mapping, which then scores 0 for every
  answer.
- Item validation does not check that each interaction's `response-identifier` is declared.

## License

MIT. See `LICENSE`.
