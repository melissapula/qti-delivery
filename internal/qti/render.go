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
