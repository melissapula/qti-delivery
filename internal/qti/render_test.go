package qti

import (
	"encoding/json"
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
