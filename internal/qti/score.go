package qti

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Values of the base-type and cardinality attributes that change how a
// response is scored.
const (
	baseTypeString    = "string"
	cardinalitySingle = "single"
)

// Responses holds a candidate's answers to one item, keyed by response
// identifier. A single-answer response is a one-element slice.
type Responses map[string][]string

// Result is the outcome of scoring one item.
type Result struct {
	ItemIdentifier string
	Score          float64
	MaxScore       float64
	Correct        bool
}

// Score scores the candidate's responses to the item. An unanswered item
// scores 0. It returns an error if responses names a response the item does
// not declare, or gives more than one value to a single-answer response.
func (it *Item) Score(responses Responses) (Result, error) {
	for id := range responses {
		if !it.declares(id) {
			return Result{}, fmt.Errorf("item %q: unknown response %q", it.Identifier, id)
		}
	}

	result := Result{ItemIdentifier: it.Identifier, MaxScore: it.MaxScore()}
	for _, decl := range it.ResponseDeclarations {
		values := normalize(responses[decl.Identifier])
		if decl.Cardinality == cardinalitySingle && len(values) > 1 {
			return Result{}, fmt.Errorf("item %q: response %q takes one value, got %d", it.Identifier, decl.Identifier, len(values))
		}

		switch name := it.TemplateName(); name {
		case templateMatchCorrect:
			if matchCorrect(decl, values) {
				result.Score++
			}
		case templateMapResponse:
			result.Score += mapResponse(decl, values)
		default:
			return Result{}, fmt.Errorf("item %q: unsupported scoring template %q", it.Identifier, name)
		}
	}
	result.Correct = result.MaxScore > 0 && result.Score >= result.MaxScore
	return result, nil
}

// MaxScore returns the highest score a candidate can earn on the item.
func (it *Item) MaxScore() float64 {
	var total float64
	for _, decl := range it.ResponseDeclarations {
		switch it.TemplateName() {
		case templateMatchCorrect:
			total++
		case templateMapResponse:
			total += maxMapped(decl)
		}
	}
	return total
}

// declares reports whether the item has a response declaration named id.
func (it *Item) declares(id string) bool {
	for _, decl := range it.ResponseDeclarations {
		if decl.Identifier == id {
			return true
		}
	}
	return false
}

// maxMapped returns the highest total the declaration's mapping can award:
// the largest mapped value for a single-answer response, or the sum of the
// positive mapped values when several answers are allowed. Either is capped
// by the mapping's upper bound.
func maxMapped(decl ResponseDeclaration) float64 {
	m := decl.Mapping
	if m == nil {
		return 0
	}

	var best float64
	for _, e := range m.Entries {
		if e.MappedValue <= 0 {
			continue
		}
		if decl.Cardinality == cardinalitySingle {
			best = max(best, e.MappedValue)
		} else {
			best += e.MappedValue
		}
	}
	return clamp(best, nil, m.UpperBound)
}

// normalize trims whitespace from each value, then drops empty values and
// duplicates. Values keep the order in which they first appear.
func normalize(values []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// matchCorrect reports whether values are exactly the declaration's correct
// response: the same values, in any order. Strings are compared without
// regard to case; every other base type must match exactly. It expects
// values to be normalized already.
func matchCorrect(decl ResponseDeclaration, values []string) bool {
	foldCase := decl.BaseType == baseTypeString
	want := sortedCopy(normalize(decl.CorrectResponse), foldCase)
	if len(want) == 0 {
		return false
	}
	return slices.Equal(sortedCopy(values, foldCase), want)
}

// sortedCopy returns a sorted copy of values, lower-cased first if foldCase
// is set. It copies so that the caller's slice is not reordered.
func sortedCopy(values []string, foldCase bool) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if foldCase {
			v = strings.ToLower(v)
		}
		out[i] = v
	}
	sort.Strings(out)
	return out
}

// clamp limits score to the range from lower to upper. A nil bound means
// there is no limit on that side.
func clamp(score float64, lower, upper *float64) float64 {
	if lower != nil && score < *lower {
		return *lower
	}
	if upper != nil && score > *upper {
		return *upper
	}
	return score
}

// mapResponse scores values against the declaration's mapping. It adds up
// the mapped value of each answer, using the default value for an answer
// with no entry, and clamps the total to the mapping's bounds. An entry
// counts once, however many answers match it. A blank response scores 0. It
// expects values to be normalized already.
func mapResponse(decl ResponseDeclaration, values []string) float64 {
	m := decl.Mapping
	if m == nil || len(values) == 0 {
		return 0
	}

	var total float64
	used := make(map[int]bool) // index of each entry already counted
	for _, v := range values {
		i := m.entryIndex(v)
		if i < 0 {
			total += m.DefaultValue
			continue
		}
		if !used[i] {
			used[i] = true
			total += m.Entries[i].MappedValue
		}
	}
	return clamp(total, m.LowerBound, m.UpperBound)
}

// entryIndex returns the index of the first entry whose key matches value,
// or -1 if there is none.
func (m *Mapping) entryIndex(value string) int {
	for i, e := range m.Entries {
		if e.matches(value) {
			return i
		}
	}
	return -1
}

// matches reports whether value equals the entry's key. The comparison is
// case-sensitive unless the entry sets case-sensitive to false.
func (e MapEntry) matches(value string) bool {
	if e.CaseSensitive != nil && !*e.CaseSensitive {
		return strings.EqualFold(e.MapKey, value)
	}
	return e.MapKey == value
}
