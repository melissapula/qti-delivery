package qti

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"empty slice", []string{}, nil},
		{"single value", []string{"B"}, []string{"B"}},
		{"keeps order", []string{"C", "A", "D"}, []string{"C", "A", "D"}},
		{"trims whitespace", []string{"  B ", "\tC\n"}, []string{"B", "C"}},
		{"drops empty and blank", []string{"", "A", "   "}, []string{"A"}},
		{"only blanks", []string{"", " "}, nil},
		{"drops duplicates", []string{"A", "A"}, []string{"A"}},
		{"duplicate after trimming", []string{"A", " A "}, []string{"A"}},
		{"case is significant", []string{"a", "A"}, []string{"a", "A"}},
		{"keeps inner spaces", []string{" Lake Itasca "}, []string{"Lake Itasca"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMatchCorrect(t *testing.T) {
	single := ResponseDeclaration{
		Cardinality:     "single",
		BaseType:        "identifier",
		CorrectResponse: []string{"B"},
	}
	multiple := ResponseDeclaration{
		Cardinality:     "multiple",
		BaseType:        "identifier",
		CorrectResponse: []string{"A", "C", "D"},
	}
	text := ResponseDeclaration{
		Cardinality:     "single",
		BaseType:        "string",
		CorrectResponse: []string{"Itasca"},
	}
	textMultiple := ResponseDeclaration{
		Cardinality:     "multiple",
		BaseType:        "string",
		CorrectResponse: []string{"b", "A"},
	}
	noCorrect := ResponseDeclaration{
		Cardinality: "single",
		BaseType:    "identifier",
	}

	tests := []struct {
		name   string
		decl   ResponseDeclaration
		values []string
		want   bool
	}{
		{"single: correct", single, []string{"B"}, true},
		{"single: wrong", single, []string{"A"}, false},
		{"single: blank", single, nil, false},
		{"single: correct plus another", single, []string{"B", "A"}, false},
		{"single: identifiers are case-sensitive", single, []string{"b"}, false},

		{"multiple: all correct", multiple, []string{"A", "C", "D"}, true},
		{"multiple: any order", multiple, []string{"D", "A", "C"}, true},
		{"multiple: one missing", multiple, []string{"A", "C"}, false},
		{"multiple: one extra", multiple, []string{"A", "B", "C", "D"}, false},
		{"multiple: one swapped", multiple, []string{"A", "B", "D"}, false},
		{"multiple: blank", multiple, nil, false},

		{"string: exact", text, []string{"Itasca"}, true},
		{"string: lower case", text, []string{"itasca"}, true},
		{"string: upper case", text, []string{"ITASCA"}, true},
		{"string: wrong", text, []string{"Superior"}, false},
		{"string: blank", text, nil, false},
		{"string multiple: other case and order", textMultiple, []string{"a", "B"}, true},

		{"no correct response declared", noCorrect, []string{"B"}, false},
		{"no correct response, blank answer", noCorrect, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchCorrect(tt.decl, tt.values); got != tt.want {
				t.Errorf("matchCorrect(%q) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

// TestMatchCorrectKeepsOrder checks that comparing does not reorder the
// caller's slices.
func TestMatchCorrectKeepsOrder(t *testing.T) {
	decl := ResponseDeclaration{
		Cardinality:     "multiple",
		BaseType:        "identifier",
		CorrectResponse: []string{"D", "A", "C"},
	}
	values := []string{"C", "D", "A"}

	matchCorrect(decl, values)

	if want := []string{"C", "D", "A"}; !slices.Equal(values, want) {
		t.Errorf("values = %q after matchCorrect, want %q", values, want)
	}
	if want := []string{"D", "A", "C"}; !slices.Equal(decl.CorrectResponse, want) {
		t.Errorf("CorrectResponse = %q after matchCorrect, want %q", decl.CorrectResponse, want)
	}
}

// ptr returns a pointer to f, for building optional bounds in tests.
func ptr(f float64) *float64 {
	return &f
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		lower *float64
		upper *float64
		want  float64
	}{
		{"no bounds: positive", 5, nil, nil, 5},
		{"no bounds: negative", -2, nil, nil, -2},

		{"lower only: below", -1, ptr(0), nil, 0},
		{"lower only: above", 2, ptr(0), nil, 2},
		{"lower only: negative bound", -5, ptr(-2), nil, -2},

		{"upper only: above", 5, nil, ptr(3), 3},
		{"upper only: below", 2, nil, ptr(3), 2},
		{"upper only: another bound", 5, nil, ptr(1), 1},

		{"both: below lower", -1, ptr(0), ptr(3), 0},
		{"both: above upper", 4, ptr(0), ptr(3), 3},
		{"both: inside", 1.5, ptr(0), ptr(3), 1.5},
		{"both: on lower", 0, ptr(0), ptr(3), 0},
		{"both: on upper", 3, ptr(0), ptr(3), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clamp(tt.score, tt.lower, tt.upper); got != tt.want {
				t.Errorf("clamp(%v) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

// boolPtr returns a pointer to b, for building optional flags in tests.
func boolPtr(b bool) *bool {
	return &b
}

// sampleDecl loads a sample item and returns its only response declaration.
func sampleDecl(t *testing.T, file string) ResponseDeclaration {
	t.Helper()
	item, err := ParseFile(filepath.Join(itemsDir, file))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(item.ResponseDeclarations) != 1 {
		t.Fatalf("%s: got %d response declarations, want 1", file, len(item.ResponseDeclarations))
	}
	return item.ResponseDeclarations[0]
}

func TestMapResponse(t *testing.T) {
	primes := sampleDecl(t, "02-primes.xml")
	lake := sampleDecl(t, "03-lake.xml")

	// withMapping builds a declaration around a mapping written in the test.
	withMapping := func(m Mapping) ResponseDeclaration {
		return ResponseDeclaration{Cardinality: "multiple", BaseType: "string", Mapping: &m}
	}
	withDefault := withMapping(Mapping{
		DefaultValue: 0.25,
		Entries:      []MapEntry{{MapKey: "A", MappedValue: 1}},
	})
	capped := withMapping(Mapping{
		UpperBound: ptr(3),
		Entries:    []MapEntry{{MapKey: "A", MappedValue: 2}, {MapKey: "B", MappedValue: 2}},
	})
	unbounded := withMapping(Mapping{
		Entries: []MapEntry{{MapKey: "B", MappedValue: -1}},
	})
	raisedFloor := withMapping(Mapping{
		LowerBound: ptr(1),
		Entries:    []MapEntry{{MapKey: "A", MappedValue: 2}},
	})
	caseDefault := withMapping(Mapping{
		Entries: []MapEntry{{MapKey: "Paris", MappedValue: 1}},
	})
	caseTrue := withMapping(Mapping{
		Entries: []MapEntry{{MapKey: "Paris", MappedValue: 1, CaseSensitive: boolPtr(true)}},
	})
	caseFalse := withMapping(Mapping{
		Entries: []MapEntry{{MapKey: "Paris", MappedValue: 1, CaseSensitive: boolPtr(false)}},
	})
	noMapping := ResponseDeclaration{Cardinality: "single", BaseType: "identifier"}

	tests := []struct {
		name   string
		decl   ResponseDeclaration
		values []string
		want   float64
	}{
		{"primes: all three", primes, []string{"A", "C", "D"}, 3},
		{"primes: one right", primes, []string{"A"}, 1},
		{"primes: two right", primes, []string{"A", "C"}, 2},
		{"primes: right and wrong cancel", primes, []string{"A", "B"}, 0},
		{"primes: wrong alone is clamped to 0", primes, []string{"B"}, 0},
		{"primes: everything selected", primes, []string{"A", "B", "C", "D"}, 2},
		{"primes: unknown choice", primes, []string{"Z"}, 0},
		{"primes: blank", primes, nil, 0},

		{"lake: exact", lake, []string{"Itasca"}, 1},
		{"lake: lower case", lake, []string{"itasca"}, 1},
		{"lake: upper case, second entry", lake, []string{"LAKE ITASCA"}, 1},
		{"lake: wrong", lake, []string{"Superior"}, 0},
		{"lake: blank", lake, nil, 0},

		{"default value: one unmapped answer", withDefault, []string{"X"}, 0.25},
		{"default value: two unmapped answers", withDefault, []string{"X", "Y"}, 0.5},
		{"default value: mapped plus unmapped", withDefault, []string{"A", "X"}, 1.25},

		{"upper bound caps the total", capped, []string{"A", "B"}, 3},
		{"upper bound: under the cap", capped, []string{"A"}, 2},
		{"no bounds: negative total stands", unbounded, []string{"B"}, -1},
		{"lower bound raises a low total", raisedFloor, []string{"X"}, 1},
		{"blank is 0 even with a lower bound above 0", raisedFloor, nil, 0},

		{"case-sensitive by default: exact", caseDefault, []string{"Paris"}, 1},
		{"case-sensitive by default: other case", caseDefault, []string{"paris"}, 0},
		{"case-sensitive true: other case", caseTrue, []string{"paris"}, 0},
		{"case-sensitive false: other case", caseFalse, []string{"PARIS"}, 1},
		{"one entry matched twice counts once", caseFalse, []string{"paris", "PARIS"}, 1},

		{"no mapping", noMapping, []string{"A"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapResponse(tt.decl, tt.values); got != tt.want {
				t.Errorf("mapResponse(%q) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

// sampleItem loads one of the sample items.
func sampleItem(t *testing.T, file string) *Item {
	t.Helper()
	item, err := ParseFile(filepath.Join(itemsDir, file))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return item
}

// mappedItem builds a map_response item around a mapping written in a test.
func mappedItem(cardinality string, m *Mapping) *Item {
	return &Item{
		Identifier: "test-item",
		ResponseDeclarations: []ResponseDeclaration{
			{Identifier: "RESPONSE", Cardinality: cardinality, BaseType: "identifier", Mapping: m},
		},
		ResponseProcessing: ResponseProcessing{Template: "map_response"},
	}
}

func TestMaxScore(t *testing.T) {
	// entries builds map entries keyed A, B, C and so on.
	entries := func(values ...float64) []MapEntry {
		var out []MapEntry
		for i, v := range values {
			out = append(out, MapEntry{MapKey: string(rune('A' + i)), MappedValue: v})
		}
		return out
	}

	tests := []struct {
		name string
		item *Item
		want float64
	}{
		{"capital: match_correct", sampleItem(t, "01-capital.xml"), 1},
		{"primes: three positives, capped at 3", sampleItem(t, "02-primes.xml"), 3},
		{"lake: single, best entry", sampleItem(t, "03-lake.xml"), 1},

		{"single: highest value", mappedItem("single", &Mapping{Entries: entries(1, 2, 0.5)}), 2},
		{"single: capped by upper bound", mappedItem("single", &Mapping{UpperBound: ptr(2), Entries: entries(5)}), 2},
		{"single: all negative", mappedItem("single", &Mapping{Entries: entries(-1, -2)}), 0},
		{"multiple: sum of positives", mappedItem("multiple", &Mapping{Entries: entries(1, -1, 2)}), 3},
		{"multiple: capped by upper bound", mappedItem("multiple", &Mapping{UpperBound: ptr(3), Entries: entries(2, 2)}), 3},
		{"multiple: under the upper bound", mappedItem("multiple", &Mapping{UpperBound: ptr(9), Entries: entries(2, 2)}), 4},
		{"no mapping", mappedItem("single", nil), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.MaxScore(); got != tt.want {
				t.Errorf("MaxScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestScore runs the sample items through Score with the answers a candidate
// might give. A row expects either a full Result or, if wantErr is set, an
// error containing that text.
func TestScore(t *testing.T) {
	const (
		capital = "01-capital.xml"
		primes  = "02-primes.xml"
		lake    = "03-lake.xml"
	)
	// answer builds the responses for an item with one response declaration.
	answer := func(values ...string) Responses {
		return Responses{"RESPONSE": values}
	}

	tests := []struct {
		name      string
		file      string
		responses Responses
		want      Result
		wantErr   string
	}{
		// Capital: single choice, match_correct, worth 1.
		{name: "capital: correct", file: capital, responses: answer("B"),
			want: Result{"capital-mn", 1, 1, true}},
		{name: "capital: wrong", file: capital, responses: answer("A"),
			want: Result{"capital-mn", 0, 1, false}},
		{name: "capital: no responses at all", file: capital, responses: nil,
			want: Result{"capital-mn", 0, 1, false}},
		{name: "capital: response with no values", file: capital, responses: answer(),
			want: Result{"capital-mn", 0, 1, false}},
		{name: "capital: only blank values", file: capital, responses: answer("", "  "),
			want: Result{"capital-mn", 0, 1, false}},
		{name: "capital: surrounding whitespace", file: capital, responses: answer(" B "),
			want: Result{"capital-mn", 1, 1, true}},
		{name: "capital: duplicate value counts once", file: capital, responses: answer("B", "B"),
			want: Result{"capital-mn", 1, 1, true}},
		{name: "capital: identifiers are case-sensitive", file: capital, responses: answer("b"),
			want: Result{"capital-mn", 0, 1, false}},
		{name: "capital: too many values", file: capital, responses: answer("B", "A"),
			wantErr: "takes one value, got 2"},

		// Primes: multiple choice, map_response, bounds 0 to 3, worth 3.
		{name: "primes: all correct", file: primes, responses: answer("A", "C", "D"),
			want: Result{"primes", 3, 3, true}},
		{name: "primes: all correct in another order", file: primes, responses: answer("D", "C", "A"),
			want: Result{"primes", 3, 3, true}},
		{name: "primes: partial credit, one", file: primes, responses: answer("A"),
			want: Result{"primes", 1, 3, false}},
		{name: "primes: partial credit, two", file: primes, responses: answer("A", "C"),
			want: Result{"primes", 2, 3, false}},
		{name: "primes: right and wrong cancel", file: primes, responses: answer("A", "B"),
			want: Result{"primes", 0, 3, false}},
		{name: "primes: wrong alone is clamped to the lower bound", file: primes, responses: answer("B"),
			want: Result{"primes", 0, 3, false}},
		{name: "primes: everything selected", file: primes, responses: answer("A", "B", "C", "D"),
			want: Result{"primes", 2, 3, false}},
		{name: "primes: duplicate value counts once", file: primes, responses: answer("A", "A"),
			want: Result{"primes", 1, 3, false}},
		{name: "primes: whitespace and blanks ignored", file: primes, responses: answer(" A ", "", "C"),
			want: Result{"primes", 2, 3, false}},
		{name: "primes: blank", file: primes, responses: nil,
			want: Result{"primes", 0, 3, false}},

		// Lake: text entry, map_response, case-insensitive entries, worth 1.
		{name: "lake: exact", file: lake, responses: answer("Itasca"),
			want: Result{"lake-source", 1, 1, true}},
		{name: "lake: lower case", file: lake, responses: answer("itasca"),
			want: Result{"lake-source", 1, 1, true}},
		{name: "lake: second accepted answer, upper case", file: lake, responses: answer("LAKE ITASCA"),
			want: Result{"lake-source", 1, 1, true}},
		{name: "lake: surrounding whitespace", file: lake, responses: answer("  Lake Itasca  "),
			want: Result{"lake-source", 1, 1, true}},
		{name: "lake: wrong", file: lake, responses: answer("Superior"),
			want: Result{"lake-source", 0, 1, false}},
		{name: "lake: blank", file: lake, responses: answer(""),
			want: Result{"lake-source", 0, 1, false}},
		{name: "lake: too many values", file: lake, responses: answer("Itasca", "Lake Itasca"),
			wantErr: "takes one value, got 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sampleItem(t, tt.file).Score(tt.responses)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Score succeeded with %+v, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Score: %v", err)
			}
			if got != tt.want {
				t.Errorf("Score() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestScoreForm scores a perfect attempt at the whole sample form, which the
// spec's example session expects to total 5 of 5.
func TestScoreForm(t *testing.T) {
	items, err := LoadDir(itemsDir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	answers := map[string]Responses{
		"capital-mn":  {"RESPONSE": {"B"}},
		"primes":      {"RESPONSE": {"A", "C", "D"}},
		"lake-source": {"RESPONSE": {"itasca"}},
	}

	var score, maxScore float64
	for _, item := range items {
		result, err := item.Score(answers[item.Identifier])
		if err != nil {
			t.Fatalf("Score(%s): %v", item.Identifier, err)
		}
		if !result.Correct {
			t.Errorf("%s: Correct = false, want true", item.Identifier)
		}
		score += result.Score
		maxScore += result.MaxScore
	}
	if score != 5 || maxScore != 5 {
		t.Errorf("form total = %v of %v, want 5 of 5", score, maxScore)
	}
}

func TestScoreErrors(t *testing.T) {
	unsupported := &Item{
		Identifier: "odd-template",
		ResponseDeclarations: []ResponseDeclaration{
			{Identifier: "RESPONSE", Cardinality: "single", BaseType: "identifier"},
		},
		ResponseProcessing: ResponseProcessing{Template: "map_response_point"},
	}

	tests := []struct {
		name      string
		item      *Item
		responses Responses
		wantErr   string
	}{
		{
			name:      "two values for a single-answer choice",
			item:      sampleItem(t, "01-capital.xml"),
			responses: Responses{"RESPONSE": {"B", "A"}},
			wantErr:   `response "RESPONSE" takes one value, got 2`,
		},
		{
			name:      "two values for a single-answer text entry",
			item:      sampleItem(t, "03-lake.xml"),
			responses: Responses{"RESPONSE": {"Itasca", "Superior"}},
			wantErr:   `response "RESPONSE" takes one value, got 2`,
		},
		{
			name:      "unknown response identifier",
			item:      sampleItem(t, "01-capital.xml"),
			responses: Responses{"RESPONS": {"B"}},
			wantErr:   `unknown response "RESPONS"`,
		},
		{
			name:      "unsupported template",
			item:      unsupported,
			responses: Responses{"RESPONSE": {"B"}},
			wantErr:   `unsupported scoring template "map_response_point"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.item.Score(tt.responses)
			if err == nil {
				t.Fatalf("Score succeeded with %+v, want error containing %q", got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if got != (Result{}) {
				t.Errorf("Score() = %+v alongside an error, want the zero Result", got)
			}
		})
	}
}
