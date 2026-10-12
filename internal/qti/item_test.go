package qti

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// itemsDir is the sample test form, relative to this package's directory.
const itemsDir = "../../testdata/items"

func TestParseFileSamples(t *testing.T) {
	tests := []struct {
		file        string
		identifier  string
		title       string
		template    string
		cardinality string
		baseType    string
		correct     []string
		choices     int
		textEntries int
		hasMapping  bool
	}{
		{
			file:        "01-capital.xml",
			identifier:  "capital-mn",
			title:       "State Capital",
			template:    "match_correct",
			cardinality: "single",
			baseType:    "identifier",
			correct:     []string{"B"},
			choices:     4,
		},
		{
			file:        "02-primes.xml",
			identifier:  "primes",
			title:       "Prime Numbers",
			template:    "map_response",
			cardinality: "multiple",
			baseType:    "identifier",
			correct:     []string{"A", "C", "D"},
			choices:     4,
			hasMapping:  true,
		},
		{
			file:        "03-lake.xml",
			identifier:  "lake-source",
			title:       "Headwaters",
			template:    "map_response",
			cardinality: "single",
			baseType:    "string",
			correct:     []string{"Itasca"},
			textEntries: 1,
			hasMapping:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			item, err := ParseFile(filepath.Join(itemsDir, tt.file))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}

			if item.Identifier != tt.identifier {
				t.Errorf("Identifier = %q, want %q", item.Identifier, tt.identifier)
			}
			if item.Title != tt.title {
				t.Errorf("Title = %q, want %q", item.Title, tt.title)
			}
			if got := item.TemplateName(); got != tt.template {
				t.Errorf("TemplateName() = %q, want %q", got, tt.template)
			}

			if len(item.ResponseDeclarations) != 1 {
				t.Fatalf("got %d response declarations, want 1", len(item.ResponseDeclarations))
			}
			decl := item.ResponseDeclarations[0]
			if decl.Identifier != "RESPONSE" {
				t.Errorf("declaration Identifier = %q, want %q", decl.Identifier, "RESPONSE")
			}
			if decl.Cardinality != tt.cardinality {
				t.Errorf("Cardinality = %q, want %q", decl.Cardinality, tt.cardinality)
			}
			if decl.BaseType != tt.baseType {
				t.Errorf("BaseType = %q, want %q", decl.BaseType, tt.baseType)
			}
			if !slices.Equal(decl.CorrectResponse, tt.correct) {
				t.Errorf("CorrectResponse = %v, want %v", decl.CorrectResponse, tt.correct)
			}
			if got := decl.Mapping != nil; got != tt.hasMapping {
				t.Errorf("has mapping = %v, want %v", got, tt.hasMapping)
			}

			choices := 0
			for _, ci := range item.Body.ChoiceInteractions {
				choices += len(ci.Choices)
			}
			if choices != tt.choices {
				t.Errorf("got %d choices, want %d", choices, tt.choices)
			}
			textEntries := 0
			for _, p := range item.Body.Paragraphs {
				textEntries += len(p.TextEntries)
			}
			if textEntries != tt.textEntries {
				t.Errorf("got %d text entries, want %d", textEntries, tt.textEntries)
			}
		})
	}
}

func TestParseChoiceInteraction(t *testing.T) {
	item, err := ParseFile(filepath.Join(itemsDir, "01-capital.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(item.Body.ChoiceInteractions) != 1 {
		t.Fatalf("got %d choice interactions, want 1", len(item.Body.ChoiceInteractions))
	}
	ci := item.Body.ChoiceInteractions[0]

	if ci.ResponseIdentifier != "RESPONSE" {
		t.Errorf("ResponseIdentifier = %q, want %q", ci.ResponseIdentifier, "RESPONSE")
	}
	if ci.MaxChoices != 1 {
		t.Errorf("MaxChoices = %d, want 1", ci.MaxChoices)
	}
	if want := "What is the capital of Minnesota?"; ci.Prompt != want {
		t.Errorf("Prompt = %q, want %q", ci.Prompt, want)
	}
	want := []SimpleChoice{
		{Identifier: "A", Text: "Minneapolis"},
		{Identifier: "B", Text: "Saint Paul"},
		{Identifier: "C", Text: "Duluth"},
		{Identifier: "D", Text: "Bemidji"},
	}
	if !slices.Equal(ci.Choices, want) {
		t.Errorf("Choices = %v, want %v", ci.Choices, want)
	}
}

func TestParseTextEntryInteraction(t *testing.T) {
	item, err := ParseFile(filepath.Join(itemsDir, "03-lake.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(item.Body.Paragraphs) != 1 {
		t.Fatalf("got %d paragraphs, want 1", len(item.Body.Paragraphs))
	}
	p := item.Body.Paragraphs[0]

	for _, want := range []string{"The Mississippi River begins at Lake", "in northern Minnesota."} {
		if !strings.Contains(p.Text, want) {
			t.Errorf("paragraph text %q does not contain %q", p.Text, want)
		}
	}
	if len(p.TextEntries) != 1 {
		t.Fatalf("got %d text entries, want 1", len(p.TextEntries))
	}
	want := TextEntryInteraction{ResponseIdentifier: "RESPONSE", ExpectedLength: 15}
	if p.TextEntries[0] != want {
		t.Errorf("text entry = %+v, want %+v", p.TextEntries[0], want)
	}
}

// TestParseMapping checks the optional attributes: a bound or case-sensitive
// flag that is absent in the XML must be nil, not zero or false.
func TestParseMapping(t *testing.T) {
	t.Run("bounds present", func(t *testing.T) {
		item, err := ParseFile(filepath.Join(itemsDir, "02-primes.xml"))
		if err != nil {
			t.Fatalf("ParseFile: %v", err)
		}
		m := item.ResponseDeclarations[0].Mapping
		if m == nil {
			t.Fatal("Mapping is nil")
		}

		if m.LowerBound == nil || *m.LowerBound != 0 {
			t.Errorf("LowerBound = %v, want pointer to 0", m.LowerBound)
		}
		if m.UpperBound == nil || *m.UpperBound != 3 {
			t.Errorf("UpperBound = %v, want pointer to 3", m.UpperBound)
		}

		wantValues := map[string]float64{"A": 1, "B": -1, "C": 1, "D": 1}
		if len(m.Entries) != len(wantValues) {
			t.Fatalf("got %d entries, want %d", len(m.Entries), len(wantValues))
		}
		for _, e := range m.Entries {
			if e.MappedValue != wantValues[e.MapKey] {
				t.Errorf("entry %q MappedValue = %v, want %v", e.MapKey, e.MappedValue, wantValues[e.MapKey])
			}
			if e.CaseSensitive != nil {
				t.Errorf("entry %q CaseSensitive = %v, want nil", e.MapKey, *e.CaseSensitive)
			}
		}
	})

	t.Run("bounds absent", func(t *testing.T) {
		item, err := ParseFile(filepath.Join(itemsDir, "03-lake.xml"))
		if err != nil {
			t.Fatalf("ParseFile: %v", err)
		}
		m := item.ResponseDeclarations[0].Mapping
		if m == nil {
			t.Fatal("Mapping is nil")
		}

		if m.LowerBound != nil {
			t.Errorf("LowerBound = %v, want nil", *m.LowerBound)
		}
		if m.UpperBound != nil {
			t.Errorf("UpperBound = %v, want nil", *m.UpperBound)
		}

		wantKeys := []string{"Itasca", "Lake Itasca"}
		if len(m.Entries) != len(wantKeys) {
			t.Fatalf("got %d entries, want %d", len(m.Entries), len(wantKeys))
		}
		for i, e := range m.Entries {
			if e.MapKey != wantKeys[i] {
				t.Errorf("entry %d MapKey = %q, want %q", i, e.MapKey, wantKeys[i])
			}
			if e.CaseSensitive == nil || *e.CaseSensitive {
				t.Errorf("entry %q CaseSensitive = %v, want pointer to false", e.MapKey, e.CaseSensitive)
			}
		}
	})
}

// minimalItem is the smallest item Parse accepts. The invalid cases below
// are each this item with one thing wrong.
const minimalItem = `
<qti-assessment-item identifier="x">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="identifier"/>
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="1">
      <qti-simple-choice identifier="A">a</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="match_correct"/>
</qti-assessment-item>`

func TestParseMinimalItem(t *testing.T) {
	if _, err := Parse(strings.NewReader(minimalItem)); err != nil {
		t.Fatalf("Parse(minimalItem): %v", err)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		wantErr string // text the error message must contain
	}{
		{
			name:    "empty input",
			xml:     ``,
			wantErr: "EOF",
		},
		{
			name:    "malformed XML",
			xml:     `<qti-assessment-item identifier="x"><oops`,
			wantErr: "XML syntax error",
		},
		{
			name:    "wrong root element",
			xml:     `<html></html>`,
			wantErr: "expected element type <qti-assessment-item>",
		},
		{
			name:    "attribute is not a number",
			xml:     strings.Replace(minimalItem, `max-choices="1"`, `max-choices="one"`, 1),
			wantErr: "invalid syntax",
		},
		{
			name:    "no identifier",
			xml:     strings.Replace(minimalItem, ` identifier="x"`, ``, 1),
			wantErr: "no identifier",
		},
		{
			name: "no response declaration",
			xml: `
<qti-assessment-item identifier="x">
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="1">
      <qti-simple-choice identifier="A">a</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="match_correct"/>
</qti-assessment-item>`,
			wantErr: "no response declaration",
		},
		{
			name: "no supported interaction",
			xml: `
<qti-assessment-item identifier="x">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="identifier"/>
  <qti-item-body>
    <p>Put these in order.</p>
    <qti-order-interaction response-identifier="RESPONSE"/>
  </qti-item-body>
  <qti-response-processing template="match_correct"/>
</qti-assessment-item>`,
			wantErr: "no supported interaction",
		},
		{
			name:    "unsupported template",
			xml:     strings.Replace(minimalItem, `template="match_correct"`, `template="map_response_point"`, 1),
			wantErr: `unsupported scoring template "map_response_point"`,
		},
		{
			name:    "no template",
			xml:     strings.Replace(minimalItem, ` template="match_correct"`, ``, 1),
			wantErr: `unsupported scoring template ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := Parse(strings.NewReader(tt.xml))
			if err == nil {
				t.Fatalf("Parse succeeded with item %q, want error containing %q", item.Identifier, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestTemplateName(t *testing.T) {
	tests := []struct {
		template string
		want     string
	}{
		{"https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/match_correct.xml", "match_correct"},
		{"https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/map_response.xml", "map_response"},
		{"https://purl.imsglobal.org/spec/qti/v3p0/rptemplates/map_response_point.xml", "map_response_point"},
		{"match_correct", "match_correct"},
		{"map_response.xml", "map_response"},
		{"something_else", "something_else"},
		{"a/b/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			item := &Item{ResponseProcessing: ResponseProcessing{Template: tt.template}}
			if got := item.TemplateName(); got != tt.want {
				t.Errorf("TemplateName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadDir(t *testing.T) {
	t.Run("samples load in file name order", func(t *testing.T) {
		items, err := LoadDir(itemsDir)
		if err != nil {
			t.Fatalf("LoadDir: %v", err)
		}
		var got []string
		for _, item := range items {
			got = append(got, item.Identifier)
		}
		want := []string{"capital-mn", "primes", "lake-source"}
		if !slices.Equal(got, want) {
			t.Errorf("identifiers = %v, want %v", got, want)
		}
	})

	t.Run("skips files that are not .xml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "item.xml", minimalItem)
		writeFile(t, dir, "notes.txt", "not an item")

		items, err := LoadDir(dir)
		if err != nil {
			t.Fatalf("LoadDir: %v", err)
		}
		if len(items) != 1 {
			t.Errorf("got %d items, want 1", len(items))
		}
	})

	errTests := []struct {
		name    string
		files   map[string]string // file name -> content
		wantErr string
	}{
		{
			name:    "duplicate identifier",
			files:   map[string]string{"a.xml": minimalItem, "b.xml": minimalItem},
			wantErr: `duplicate identifier "x" in a.xml and b.xml`,
		},
		{
			name:    "no items",
			files:   map[string]string{"notes.txt": "not an item"},
			wantErr: "no .xml items found",
		},
		{
			name:    "invalid item names the file",
			files:   map[string]string{"good.xml": minimalItem, "bad.xml": `<html></html>`},
			wantErr: "bad.xml",
		},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}

			_, err := LoadDir(dir)
			if err == nil {
				t.Fatalf("LoadDir succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadDir(filepath.Join(t.TempDir(), "nope")); err == nil {
			t.Error("LoadDir succeeded, want error")
		}
	})
}

// writeFile creates a file in dir for a test, failing the test on error.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
