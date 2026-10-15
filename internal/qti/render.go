package qti

import "strings"

// squeeze collapses each run of whitespace in s, including newlines and
// tabs, into a single space, and removes whitespace from both ends.
func squeeze(s string) string {
	words := strings.Fields(s)
	finalString := strings.Join(words, " ")
	return finalString
}

// View is what a candidate sees of an item, ready to send as JSON. It must
// never hold correct responses or mappings: anything in it is visible in the
// browser.
type View struct {
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	// Paragraphs holds stimulus text, with a {{response-identifier}} marker
	// where each text entry sits.
	Paragraphs   []string          `json:"paragraphs,omitempty"`
	Interactions []InteractionView `json:"interactions"`
}

// The InteractionView Type values.
const (
	interactionChoice    = "choice"
	interactionTextEntry = "textEntry"
)

// InteractionView is one interaction the candidate answers. Type is "choice"
// or "textEntry". Prompt, MaxChoices and Choices apply only to a choice;
// ExpectedLength applies only to a text entry.
type InteractionView struct {
	Type               string `json:"type"`
	ResponseIdentifier string `json:"responseIdentifier"`
	Prompt             string `json:"prompt,omitempty"`
	// MaxChoices is a pointer so that 0, meaning no limit, still appears in
	// the JSON; it is nil for a text entry, which omits it.
	MaxChoices     *int         `json:"maxChoices,omitempty"`
	Choices        []ChoiceView `json:"choices,omitempty"`
	ExpectedLength int          `json:"expectedLength,omitempty"` // 0 means not given
}

// ChoiceView is one option in a choice interaction.
type ChoiceView struct {
	Identifier string `json:"identifier"`
	Text       string `json:"text"`
}

// View builds what a candidate sees of the item. It reads only the
// identifier, title and body, never the response declarations or response
// processing, so it cannot leak answers. Text is squeezed to remove XML
// indentation. Interactions list the text entries first, in paragraph order,
// then the choice interactions; the parsed item does not record how the two
// kinds were interleaved in the body.
func (it *Item) View() View {
	v := View{Identifier: it.Identifier, Title: it.Title}

	for _, p := range it.Body.Paragraphs {
		v.Paragraphs = append(v.Paragraphs, squeeze(p.Text))
		for _, te := range p.TextEntries {
			v.Interactions = append(v.Interactions, InteractionView{
				Type:               interactionTextEntry,
				ResponseIdentifier: te.ResponseIdentifier,
				ExpectedLength:     te.ExpectedLength,
			})
		}
	}

	for _, ci := range it.Body.ChoiceInteractions {
		// A fresh variable per interaction, so the view never points into
		// the item, which every session shares.
		maxChoices := ci.MaxChoices
		var choices []ChoiceView
		for _, c := range ci.Choices {
			choices = append(choices, ChoiceView{Identifier: c.Identifier, Text: squeeze(c.Text)})
		}
		v.Interactions = append(v.Interactions, InteractionView{
			Type:               interactionChoice,
			ResponseIdentifier: ci.ResponseIdentifier,
			Prompt:             squeeze(string(ci.Prompt)),
			MaxChoices:         &maxChoices,
			Choices:            choices,
		})
	}

	return v
}
