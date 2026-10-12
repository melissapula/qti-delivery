# QTI Delivery Engine (Go)

A small, production-style **assessment delivery service** written in Go. It loads test questions written in the IMS **QTI 3.0** standard, delivers them to a candidate one at a time, records their answers, and scores the attempt.

**Why this project:** It closes the Go gap on my resume with something directly relevant to K-12 assessment platforms. It shows real backend skills, not a tutorial clone: parsing a real industry standard, clean package design, a JSON HTTP API, concurrency-safe state, and thorough tests.

**Constraint:** I'm learning Go as we build this. Speed matters less than me understanding every line.

---

## How to work with me (instructions for Claude in VS Code)

Read this whole file before starting. Then follow these rules for the entire project:

1. **Teach as you build.** Before writing code for a step, explain in plain language what we're about to do and which Go concept it uses. Compare it to TypeScript, Angular or NestJS when that helps, because that's what I know best.
2. **One milestone at a time.** Finish a milestone, run its tests, then **stop and wait for me** before starting the next one. Don't build ahead.
3. **Small steps inside a milestone.** Create one file, or one function, at a time. After each one, give me a short "what just happened" summary.
4. **Let me drive sometimes.** In each milestone, pick one small function and ask me to try writing it first. Give hints before answers. Review what I wrote kindly and honestly.
5. **Standard library only.** No third-party modules unless I ask. Part of the point is learning what Go gives you out of the box.
6. **Explain errors when they happen.** If a build or test fails, show me how to read the Go error message before fixing it.
7. **Idiomatic Go.** Use `gofmt` formatting and short, clear names. Handle every error explicitly. Write doc comments on exported identifiers. Use table-driven tests.
8. **Quiz me at the end of each milestone** with 2–3 quick questions about what we just built.
9. **Keep the "Progress" checklist at the bottom of this file up to date** as we finish things.

---

## What is QTI? (background)

**QTI (Question and Test Interoperability)** is the 1EdTech (formerly IMS Global) standard for representing test questions in XML. Testing platforms use it so that items written in one system can be delivered and scored in another.

- An **item** (`qti-assessment-item`) is one question.
- A **response declaration** (`qti-response-declaration`) says what kind of answer is expected and what the correct answer is.
- The **item body** (`qti-item-body`) holds what the candidate sees: text plus one or more **interactions**, such as multiple choice or a text box.
- **Response processing** (`qti-response-processing`) says how to score the answer. QTI defines standard **templates**:
  - `match_correct`: 1 point if the answer exactly matches the correct response, otherwise 0.
  - `map_response`: each possible answer maps to a score, which allows partial credit. The total is clamped to optional lower and upper bounds.

The full spec is at https://www.imsglobal.org/spec/qti/v3p0/impl. We're implementing a **practical subset**, and that's fine. The point is to show we can work with a real spec, not to cover every feature.

---

## Scope

### In scope (v1)
- Parse QTI 3.0 items from XML files in a folder, where the folder acts as one linear "test form."
- Interactions:
  - `qti-choice-interaction`: single-answer and multiple-answer
  - `qti-text-entry-interaction`: short typed answer
- Scoring templates: `match_correct` and `map_response`, including `lower-bound`, `upper-bound`, `default-value` and `case-sensitive` on map entries.
- A JSON HTTP API to:
  - start a session
  - fetch items **without the answers**
  - save responses
  - submit and get scored results
- In-memory session store that is safe for concurrent requests.
- Structured logging, graceful shutdown, and table-driven tests throughout.

### Out of scope (v1, maybe later)
- Databases or persistence (sessions are lost on restart).
- Authentication.
- Other interaction types (ordering, matching, hotspot, extended text).
- Custom response-processing logic beyond the two templates.
- QTI test-level structure (`qti-assessment-test`, sections, branching).
- A frontend.

---

## Project layout

```
qti-delivery/
├── go.mod                      # module github.com/melissapula/qti-delivery
├── SPEC.md                     # this file
├── README.md                   # written at the end (Milestone 6)
├── cmd/
│   └── server/
│       └── main.go             # program entry point: flags, logging, HTTP server
├── internal/
│   ├── qti/                    # the QTI domain: parsing, scoring, candidate view
│   │   ├── item.go
│   │   ├── score.go
│   │   ├── render.go
│   │   └── *_test.go
│   ├── session/                # candidate attempts: start, answer, submit
│   │   ├── session.go
│   │   └── session_test.go
│   └── api/                    # HTTP handlers and JSON helpers
│       ├── api.go
│       ├── json.go
│       └── api_test.go
└── testdata/
    └── items/                  # sample QTI 3.0 item XML files
```

> **Go concept to explain:** the `internal/` directory. Go won't let code outside this module import packages under `internal/`. It's Go's built-in way to keep implementation details private. Compare it to TypeScript barrel files and `export`.

---

## Milestones

Each milestone ends with `go test ./...` passing and a pause for my review.

### Milestone 0: Setup
- Install Go 1.22 or newer from https://go.dev/dl/ and check it with `go version`.
- Install the official **Go extension for VS Code** (`golang.go`) and let it install `gopls`.
- `go mod init github.com/melissapula/qti-delivery`
- `git init`, plus a `.gitignore` for binaries.
- Write a tiny `cmd/server/main.go` that prints "hello" and run it with `go run ./cmd/server`.

**Concepts:** modules vs packages, `package main` and `func main`, `go run` vs `go build`, how `gopls` and `gofmt` work.

### Milestone 1: Model and parse a QTI item (`internal/qti/item.go`)
- Define Go structs that mirror the XML: `Item`, `ResponseDeclaration`, `Mapping`, `MapEntry`, `ItemBody`, `ChoiceInteraction`, `SimpleChoice`, `TextEntryInteraction` and `ResponseProcessing`.
- Use `encoding/xml` struct tags to map XML elements and attributes, for example `xml:"identifier,attr"` and `xml:"qti-correct-response>qti-value"`.
- `Parse(io.Reader) (*Item, error)`, `ParseFile(path)` and `LoadDir(dir)`. `LoadDir` sorts files by name and rejects duplicate identifiers.
- `Validate()` rejects items with no identifier, no response declaration, no supported interaction, or an unsupported scoring template.
- `TemplateName()` accepts either the full template URI or a bare name like `match_correct`.
- Add the sample items below to `testdata/items/`.

**Tests:** parsing each sample item, plus a table of invalid XML cases that should return errors.

**Concepts:** structs and struct tags, pointers (`*float64` for "optional" attributes, the way `?` works in TypeScript), slices, `error` values instead of exceptions, wrapping errors with `fmt.Errorf("...: %w", err)`, `defer f.Close()`.

### Milestone 2: Scoring (`internal/qti/score.go`)
- `type Responses map[string][]string` maps a response identifier to its values. Single-answer questions use a one-item slice.
- `(*Item) Score(Responses) (Result, error)` returns `Result{ItemIdentifier, Score, MaxScore, Correct}`.
- `match_correct`:
  - single answer: exact match
  - multiple answer: same set of values, in any order
  - `base-type="string"`: case-insensitive
- `map_response`:
  - sum the mapped values of each **distinct** response
  - use `default-value` for unmapped answers
  - respect `case-sensitive`, which defaults to true per the spec
  - clamp to `lower-bound` and `upper-bound`
- `MaxScore()`: for single answers, the highest mapped value; for multiple answers, the sum of positive values. Both are capped by `upper-bound`.
- Trim whitespace and ignore empty answers. Error if a single-answer response gets more than one value.

**Tests:** a big table-driven test covering correct, wrong, blank, partial credit, the clamping bounds, case-sensitivity, duplicate values and the too-many-values error.

**Concepts:** methods with pointer receivers, maps, `switch`, table-driven tests with `t.Run`, `sort.Strings`, `strings.EqualFold`.

### Milestone 3: Candidate view (`internal/qti/render.go`)
- `(*Item) View() View` returns only what a candidate should see: the title, stimulus paragraphs and interactions with their choices. It must **never** include correct answers or mappings.
- Squeeze extra whitespace left over from XML indentation.

**Tests:** marshal a view to JSON and check that the correct answer's text does **not** appear anywhere in the output. This is a security test, and it's worth explaining why it matters for high-stakes testing.

**Concepts:** separating internal models from API models, `encoding/json` struct tags, `omitempty`.

### Milestone 4: Sessions (`internal/session/session.go`)
- `Session{ID, CandidateID, Status, StartedAt, SubmittedAt, Results}`, where `Status` is `in_progress` or `submitted`.
- `Store` with `Start`, `Get`, `Answer`, `Submit` and `Items`.
- `Answer` validates the response against the item right away. It returns `ErrUnknownItem` or `ErrBadResponse`, and `ErrAlreadyDone` once the session is submitted.
- `Submit` scores every item (unanswered items score 0), totals the results, and locks the session.
- Generate IDs with `crypto/rand`.
- Protect state with `sync.RWMutex` and return **copies** so callers can't modify internal state.
- Make the clock replaceable (`now func() time.Time`) so tests can control time.

**Tests:** the happy path, answering after submit, an unknown item, a bad response, and a **concurrency test** that starts many sessions from goroutines. Run it with `go test -race ./...`.

**Concepts:** sentinel errors with `errors.Is`, mutexes, goroutines, the race detector, dependency injection with function fields.

### Milestone 5: HTTP API (`internal/api`, `cmd/server/main.go`)
Use Go 1.22+ `http.ServeMux` patterns with methods and path values (`r.PathValue("id")`). No framework.

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/healthz` | – | `{"status":"ok"}` |
| GET | `/api/items` | – | form outline: index, identifier, title, maxScore |
| POST | `/api/sessions` | `{"candidateId":"c-123"}` | 201 + session |
| GET | `/api/sessions/{id}` | – | session |
| GET | `/api/sessions/{id}/items/{index}` | – | `{index, total, item: View}` |
| PUT | `/api/sessions/{id}/responses/{itemId}` | `{"RESPONSE":["B"]}` | 204 |
| POST | `/api/sessions/{id}/submit` | – | session with results |

- Map errors to status codes:
  - not found → 404
  - already submitted → 409
  - bad response → 422
  - anything else → 500, logged
- Reject unknown JSON fields and cap request bodies at 1 MiB.
- Add request-logging middleware with `log/slog` that records method, path, status and duration.
- In `main.go`:
  - flags for `-addr` and `-items`
  - `ReadHeaderTimeout`
  - **graceful shutdown** on Ctrl+C using `signal.NotifyContext` and `srv.Shutdown`

**Tests:** `net/http/httptest` end-to-end tests that start a session, fetch an item, answer, submit and check the score. Also test each error status code.

**Concepts:** `http.Handler` and handler functions, middleware as a function that wraps a handler (compare NestJS interceptors), `context.Context`, goroutines in `main`.

### Milestone 6: Polish
- `README.md`: what it is, why QTI, how to run it, example `curl` session, design decisions and what I'd add next.
- A `Makefile` or simple scripts for `test`, `run` and `lint` (`go vet ./...`).
- GitHub Actions workflow that runs `go vet` and `go test -race ./...` on every push.
- Push to GitHub as `melissapula/qti-delivery`.

### Stretch ideas (only if I ask)
- Persist sessions in SQLite or Postgres behind an interface, so the store is swappable.
- Support `qti-assessment-test` with sections and time limits.
- Add `qti-order-interaction` and `qti-match-interaction`.
- A tiny Angular frontend that consumes the API, which plays to my strength.
- Dockerfile and deployment to Fly.io.
- Accessibility notes for the candidate view (WCAG matters a lot in K-12 testing).

---

## Sample items (`testdata/items/`)

Number the files so they load in order.

**`01-capital.xml`**: single choice, `match_correct`
```xml
<qti-assessment-item xmlns="http://www.imsglobal.org/xsd/imsqtiasi_v3p0"
    identifier="capital-mn" title="State Capital" adaptive="false" time-dependent="false">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="identifier">
    <qti-correct-response><qti-value>B</qti-value></qti-correct-response>
  </qti-response-declaration>
  <qti-outcome-declaration identifier="SCORE" cardinality="single" base-type="float"/>
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="1">
      <qti-prompt>What is the capital of Minnesota?</qti-prompt>
      <qti-simple-choice identifier="A">Minneapolis</qti-simple-choice>
      <qti-simple-choice identifier="B">Saint Paul</qti-simple-choice>
      <qti-simple-choice identifier="C">Duluth</qti-simple-choice>
      <qti-simple-choice identifier="D">Bemidji</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/match_correct.xml"/>
</qti-assessment-item>
```

**`02-primes.xml`**: multiple choice with partial credit, `map_response`
```xml
<qti-assessment-item xmlns="http://www.imsglobal.org/xsd/imsqtiasi_v3p0"
    identifier="primes" title="Prime Numbers" adaptive="false" time-dependent="false">
  <qti-response-declaration identifier="RESPONSE" cardinality="multiple" base-type="identifier">
    <qti-correct-response>
      <qti-value>A</qti-value><qti-value>C</qti-value><qti-value>D</qti-value>
    </qti-correct-response>
    <qti-mapping lower-bound="0" upper-bound="3" default-value="0">
      <qti-map-entry map-key="A" mapped-value="1"/>
      <qti-map-entry map-key="B" mapped-value="-1"/>
      <qti-map-entry map-key="C" mapped-value="1"/>
      <qti-map-entry map-key="D" mapped-value="1"/>
    </qti-mapping>
  </qti-response-declaration>
  <qti-outcome-declaration identifier="SCORE" cardinality="single" base-type="float"/>
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="0">
      <qti-prompt>Select all of the prime numbers.</qti-prompt>
      <qti-simple-choice identifier="A">2</qti-simple-choice>
      <qti-simple-choice identifier="B">9</qti-simple-choice>
      <qti-simple-choice identifier="C">11</qti-simple-choice>
      <qti-simple-choice identifier="D">13</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/map_response.xml"/>
</qti-assessment-item>
```

**`03-lake.xml`**: text entry, case-insensitive, `map_response`
```xml
<qti-assessment-item xmlns="http://www.imsglobal.org/xsd/imsqtiasi_v3p0"
    identifier="lake-source" title="Headwaters" adaptive="false" time-dependent="false">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="string">
    <qti-correct-response><qti-value>Itasca</qti-value></qti-correct-response>
    <qti-mapping default-value="0">
      <qti-map-entry map-key="Itasca" mapped-value="1" case-sensitive="false"/>
      <qti-map-entry map-key="Lake Itasca" mapped-value="1" case-sensitive="false"/>
    </qti-mapping>
  </qti-response-declaration>
  <qti-outcome-declaration identifier="SCORE" cardinality="single" base-type="float"/>
  <qti-item-body>
    <p>The Mississippi River begins at Lake
      <qti-text-entry-interaction response-identifier="RESPONSE" expected-length="15"/>
      in northern Minnesota.</p>
  </qti-item-body>
  <qti-response-processing template="https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/map_response.xml"/>
</qti-assessment-item>
```

Expected scores to check in tests:
- Capital: `B` → 1/1. `A` → 0/1. Blank → 0/1.
- Primes:
  - `A,C,D` → 3/3
  - `A,B` → 0/3 (1 − 1 = 0)
  - `B` alone → 0, because −1 is clamped to the lower bound
  - `A,A` → 1, because duplicates count once
- Lake: `itasca` → 1. `LAKE ITASCA` → 1. `Superior` → 0.

---

## Example API session (for the README and manual testing)

```bash
go run ./cmd/server -items ./testdata/items

curl -s -X POST localhost:8080/api/sessions -d '{"candidateId":"c-123"}'
# → {"id":"<SESSION>","candidateId":"c-123","status":"in_progress",...}

curl -s localhost:8080/api/sessions/<SESSION>/items/0
curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/capital-mn -d '{"RESPONSE":["B"]}'
curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/primes -d '{"RESPONSE":["A","C","D"]}'
curl -s -X PUT localhost:8080/api/sessions/<SESSION>/responses/lake-source -d '{"RESPONSE":["itasca"]}'
curl -s -X POST localhost:8080/api/sessions/<SESSION>/submit
# → results: score 5 of maxScore 5
```

On Windows PowerShell, use `curl.exe` instead of `curl`, or use `Invoke-RestMethod`.

---

## Resume line (once it's on GitHub)

> **QTI Delivery Engine** (Go, standard library only): an assessment-delivery API implementing a subset of IMS QTI 3.0. It parses items, delivers answer-free candidate views, and scores responses with the `match_correct` and `map_response` templates. Includes concurrency-safe sessions, structured logging, graceful shutdown and race-tested, table-driven tests.

---

## Progress

- [x] Milestone 0: Setup
- [ ] Milestone 1: Parse items
- [ ] Milestone 2: Scoring
- [ ] Milestone 3: Candidate view
- [ ] Milestone 4: Sessions
- [ ] Milestone 5: HTTP API
- [ ] Milestone 6: Polish and publish
