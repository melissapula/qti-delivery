package qti

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSqueeze(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"only whitespace", "  \n\t ", ""},
		{"one word", "hello", "hello"},
		{"already tidy", "Saint Paul", "Saint Paul"},
		{"leading and trailing spaces", "  hello  ", "hello"},
		{"several spaces inside", "a   b", "a b"},
		{"tabs", "a\t\tb", "a b"},
		{"newline and indentation", "a\n      b", "a b"},
		{"windows line endings", "\r\n a \r\n b \r\n", "a b"},
		{
			"lake paragraph",
			"The Mississippi River begins at Lake\n      \n      in northern Minnesota.",
			"The Mississippi River begins at Lake in northern Minnesota.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := squeeze(tt.in); got != tt.want {
				t.Errorf("squeeze(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func intPtr(n int) *int {
	return &n
}

// TestViewJSON pins the JSON names the candidate API will use, so renaming a
// field is a deliberate test change rather than a silent client break.
func TestViewJSON(t *testing.T) {
	tests := []struct {
		name string
		view View
		want string
	}{
		{
			name: "single choice keeps maxChoices 1 and omits paragraphs",
			view: View{
				Identifier: "capital-mn",
				Title:      "State Capital",
				Interactions: []InteractionView{{
					Type:               "choice",
					ResponseIdentifier: "RESPONSE",
					Prompt:             "Which city is the capital of Minnesota?",
					MaxChoices:         intPtr(1),
					Choices: []ChoiceView{
						{Identifier: "A", Text: "Minneapolis"},
						{Identifier: "B", Text: "Saint Paul"},
					},
				}},
			},
			want: `{"identifier":"capital-mn","title":"State Capital","interactions":[` +
				`{"type":"choice","responseIdentifier":"RESPONSE","prompt":"Which city is the capital of Minnesota?","maxChoices":1,` +
				`"choices":[{"identifier":"A","text":"Minneapolis"},{"identifier":"B","text":"Saint Paul"}]}]}`,
		},
		{
			name: "multiple choice keeps maxChoices 0 for no limit",
			view: View{
				Identifier: "primes",
				Title:      "Primes",
				Interactions: []InteractionView{{
					Type:               "choice",
					ResponseIdentifier: "RESPONSE",
					MaxChoices:         intPtr(0),
					Choices:            []ChoiceView{{Identifier: "A", Text: "2"}},
				}},
			},
			want: `{"identifier":"primes","title":"Primes","interactions":[` +
				`{"type":"choice","responseIdentifier":"RESPONSE","maxChoices":0,` +
				`"choices":[{"identifier":"A","text":"2"}]}]}`,
		},
		{
			name: "text entry omits maxChoices, choices and prompt",
			view: View{
				Identifier: "lake",
				Title:      "Lake",
				Paragraphs: []string{"begins at Lake {{RESPONSE}} today"},
				Interactions: []InteractionView{{
					Type:               "textEntry",
					ResponseIdentifier: "RESPONSE",
					ExpectedLength:     15,
				}},
			},
			want: `{"identifier":"lake","title":"Lake","paragraphs":["begins at Lake {{RESPONSE}} today"],"interactions":[` +
				`{"type":"textEntry","responseIdentifier":"RESPONSE","expectedLength":15}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.view)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("JSON =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// viewJSON renders v for a failure message; %v would print MaxChoices as an
// address rather than a number.
func viewJSON(t *testing.T, v View) string {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b)
}

func TestView(t *testing.T) {
	tests := []struct {
		file string
		want View
	}{
		{
			file: "01-capital.xml",
			want: View{
				Identifier: "capital-mn",
				Title:      "State Capital",
				Interactions: []InteractionView{{
					Type:               "choice",
					ResponseIdentifier: "RESPONSE",
					Prompt:             "What is the capital of Minnesota?",
					MaxChoices:         intPtr(1),
					Choices: []ChoiceView{
						{Identifier: "A", Text: "Minneapolis"},
						{Identifier: "B", Text: "Saint Paul"},
						{Identifier: "C", Text: "Duluth"},
						{Identifier: "D", Text: "Bemidji"},
					},
				}},
			},
		},
		{
			file: "02-primes.xml",
			want: View{
				Identifier: "primes",
				Title:      "Prime Numbers",
				Interactions: []InteractionView{{
					Type:               "choice",
					ResponseIdentifier: "RESPONSE",
					Prompt:             "Select all of the prime numbers.",
					MaxChoices:         intPtr(0),
					Choices: []ChoiceView{
						{Identifier: "A", Text: "2"},
						{Identifier: "B", Text: "9"},
						{Identifier: "C", Text: "11"},
						{Identifier: "D", Text: "13"},
					},
				}},
			},
		},
		{
			file: "03-lake.xml",
			want: View{
				Identifier: "lake-source",
				Title:      "Headwaters",
				Paragraphs: []string{"The Mississippi River begins at Lake {{RESPONSE}} in northern Minnesota."},
				Interactions: []InteractionView{{
					Type:               "textEntry",
					ResponseIdentifier: "RESPONSE",
					ExpectedLength:     15,
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			item, err := ParseFile(filepath.Join(itemsDir, tt.file))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			// reflect.DeepEqual follows the MaxChoices pointers and compares
			// the numbers, so nil, 0 and 1 are all told apart.
			if got := item.View(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("View() =\n%s\nwant\n%s", viewJSON(t, got), viewJSON(t, tt.want))
			}
		})
	}
}

// TestViewSqueezesText covers indented prompt and choice text, which none of
// the samples has: their prompts and choices each sit on one tidy line.
func TestViewSqueezesText(t *testing.T) {
	const src = `<qti-assessment-item identifier="squeeze" title="Squeeze">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="identifier">
    <qti-correct-response><qti-value>A</qti-value></qti-correct-response>
  </qti-response-declaration>
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="1">
      <qti-prompt>
        Pick the
        first one.
      </qti-prompt>
      <qti-simple-choice identifier="A">
        First
        choice
      </qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="match_correct"/>
</qti-assessment-item>`

	item, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	iv := item.View().Interactions[0]
	if want := "Pick the first one."; iv.Prompt != want {
		t.Errorf("Prompt = %q, want %q", iv.Prompt, want)
	}
	if want := "First choice"; iv.Choices[0].Text != want {
		t.Errorf("Choices[0].Text = %q, want %q", iv.Choices[0].Text, want)
	}
}

// TestViewKeepsInlineText covers prompts and choices with inline formatting,
// which none of the samples has. Dropping the formatted text can reverse a
// question's meaning, as "not" would here.
func TestViewKeepsInlineText(t *testing.T) {
	const src = `<qti-assessment-item identifier="inline" title="Inline">
  <qti-response-declaration identifier="RESPONSE" cardinality="single" base-type="identifier">
    <qti-correct-response><qti-value>B</qti-value></qti-correct-response>
  </qti-response-declaration>
  <qti-item-body>
    <qti-choice-interaction response-identifier="RESPONSE" max-choices="1">
      <qti-prompt>Which number is <strong>not</strong> prime?</qti-prompt>
      <qti-simple-choice identifier="A"><em>2</em></qti-simple-choice>
      <qti-simple-choice identifier="B">9</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="match_correct"/>
</qti-assessment-item>`

	item, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	iv := item.View().Interactions[0]
	if want := "Which number is not prime?"; iv.Prompt != want {
		t.Errorf("Prompt = %q, want %q", iv.Prompt, want)
	}
	want := []ChoiceView{{Identifier: "A", Text: "2"}, {Identifier: "B", Text: "9"}}
	if !slices.Equal(iv.Choices, want) {
		t.Errorf("Choices = %+v, want %+v", iv.Choices, want)
	}
}

// TestViewCopiesMaxChoices guards against the view pointing into the loaded
// item: items are shared by every session, so a caller changing a view must
// not change the item.
func TestViewCopiesMaxChoices(t *testing.T) {
	item, err := ParseFile(filepath.Join(itemsDir, "01-capital.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	v := item.View()
	*v.Interactions[0].MaxChoices = 99
	if got := item.Body.ChoiceInteractions[0].MaxChoices; got != 1 {
		t.Errorf("item MaxChoices = %d after changing the view, want 1", got)
	}
}

// viewKeys is every JSON key a View may contain. A key outside this set means
// a field was added to the view, which must be a deliberate test change.
var viewKeys = map[string]bool{
	"identifier":         true,
	"title":              true,
	"paragraphs":         true,
	"interactions":       true,
	"type":               true,
	"responseIdentifier": true,
	"prompt":             true,
	"maxChoices":         true,
	"choices":            true,
	"text":               true,
	"expectedLength":     true,
}

// collectKeys adds every object key in v, at every depth, to keys. v is a
// value decoded by encoding/json into an any.
func collectKeys(v any, keys map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			keys[k] = true
			collectKeys(child, keys)
		}
	case []any:
		for _, child := range v {
			collectKeys(child, keys)
		}
	}
}

// sentinelItem has answers made of strings and numbers that appear nowhere
// else in the item: not as choice identifiers, not in the text. If any of them
// shows up in the view's JSON, it can only have come from the response
// declarations.
const sentinelItem = `<qti-assessment-item identifier="sentinel" title="Sentinel">
  <qti-response-declaration identifier="CHOICE" cardinality="multiple" base-type="identifier">
    <qti-correct-response><qti-value>SECRET-CORRECT-1</qti-value></qti-correct-response>
    <qti-mapping lower-bound="-9.75" upper-bound="42.5" default-value="0.125">
      <qti-map-entry map-key="SECRET-KEY-1" mapped-value="7.25"/>
      <qti-map-entry map-key="SECRET-KEY-2" mapped-value="-3.5"/>
    </qti-mapping>
  </qti-response-declaration>
  <qti-response-declaration identifier="TEXT" cardinality="single" base-type="string">
    <qti-correct-response><qti-value>SECRET-CORRECT-2</qti-value></qti-correct-response>
    <qti-mapping default-value="0">
      <qti-map-entry map-key="SECRET-KEY-3" mapped-value="6.5" case-sensitive="false"/>
    </qti-mapping>
  </qti-response-declaration>
  <qti-item-body>
    <p>Fill in <qti-text-entry-interaction response-identifier="TEXT" expected-length="20"/> here.</p>
    <qti-choice-interaction response-identifier="CHOICE" max-choices="0">
      <qti-prompt>Pick any.</qti-prompt>
      <qti-simple-choice identifier="A">Alpha</qti-simple-choice>
      <qti-simple-choice identifier="B">Beta</qti-simple-choice>
    </qti-choice-interaction>
  </qti-item-body>
  <qti-response-processing template="map_response"/>
</qti-assessment-item>`

// TestViewDoesNotLeakAnswers is a security test. The view is sent to the
// candidate's browser, where anything in the payload can be read in the
// developer tools, so a correct response or mapping in it would hand the
// answers to anyone who looks. In high-stakes testing that invalidates the
// result for every candidate who saw the item, not only the one who looked.
//
// Searching the samples for their correct answers is not enough on its own:
// a choice item's correct answer, such as "B", is also a choice identifier
// the candidate must see. So the test checks two things: that the JSON holds
// no key outside the known view fields, and that a sentinel item's secrets,
// which appear nowhere else, never reach the JSON.
func TestViewDoesNotLeakAnswers(t *testing.T) {
	samples := []struct {
		file string
		// secrets appear only in the item's response declaration, so they
		// can be searched for safely.
		secrets []string
	}{
		{file: "01-capital.xml"},
		{file: "02-primes.xml"},
		{file: "03-lake.xml", secrets: []string{"Itasca", "Lake Itasca"}},
	}

	for _, tt := range samples {
		t.Run(tt.file, func(t *testing.T) {
			item, err := ParseFile(filepath.Join(itemsDir, tt.file))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			checkNoLeak(t, item, tt.secrets)
		})
	}

	t.Run("sentinel", func(t *testing.T) {
		item, err := Parse(strings.NewReader(sentinelItem))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		checkNoLeak(t, item, []string{
			"SECRET-CORRECT-1", "SECRET-CORRECT-2",
			"SECRET-KEY-1", "SECRET-KEY-2", "SECRET-KEY-3",
			"7.25", "-3.5", "6.5", "42.5", "-9.75", "0.125",
		})
	})
}

// checkNoLeak marshals the item's view and fails if the JSON holds a key
// outside viewKeys or contains any of secrets.
func checkNoLeak(t *testing.T, item *Item, secrets []string) {
	t.Helper()
	b, err := json.Marshal(item.View())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	keys := make(map[string]bool)
	collectKeys(decoded, keys)
	for k := range keys {
		if !viewKeys[k] {
			t.Errorf("view JSON has unexpected key %q:\n%s", k, b)
		}
	}

	for _, s := range secrets {
		if strings.Contains(string(b), s) {
			t.Errorf("view JSON contains secret %q:\n%s", s, b)
		}
	}
}
