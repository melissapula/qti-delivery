package qti

import (
	"encoding/json"
	"path/filepath"
	"reflect"
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
