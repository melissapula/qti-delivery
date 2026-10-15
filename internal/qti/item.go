// Package qti models, parses and scores a subset of IMS QTI 3.0 assessment items.
package qti

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Item is one question: a qti-assessment-item element.
type Item struct {
	XMLName              xml.Name              `xml:"qti-assessment-item"`
	Identifier           string                `xml:"identifier,attr"`
	Title                string                `xml:"title,attr"`
	ResponseDeclarations []ResponseDeclaration `xml:"qti-response-declaration"`
	Body                 ItemBody              `xml:"qti-item-body"`
	ResponseProcessing   ResponseProcessing    `xml:"qti-response-processing"`
}

// ResponseDeclaration describes the answer an interaction expects and what
// the correct answer is.
type ResponseDeclaration struct {
	Identifier      string   `xml:"identifier,attr"`
	Cardinality     string   `xml:"cardinality,attr"`
	BaseType        string   `xml:"base-type,attr"`
	CorrectResponse []string `xml:"qti-correct-response>qti-value"`
	Mapping         *Mapping `xml:"qti-mapping"`
}

// Mapping assigns a score to each possible answer, which allows partial credit.
// LowerBound and UpperBound are nil when the attribute is absent.
type Mapping struct {
	LowerBound   *float64   `xml:"lower-bound,attr"`
	UpperBound   *float64   `xml:"upper-bound,attr"`
	DefaultValue float64    `xml:"default-value,attr"`
	Entries      []MapEntry `xml:"qti-map-entry"`
}

// MapEntry maps one answer to its score. CaseSensitive is nil when the
// attribute is absent, which the spec treats as true.
type MapEntry struct {
	MapKey        string  `xml:"map-key,attr"`
	MappedValue   float64 `xml:"mapped-value,attr"`
	CaseSensitive *bool   `xml:"case-sensitive,attr"`
}

// ItemBody holds what the candidate sees: text and interactions.
type ItemBody struct {
	Paragraphs         []Paragraph         `xml:"p"`
	ChoiceInteractions []ChoiceInteraction `xml:"qti-choice-interaction"`
}

// Paragraph is a block of stimulus text, which may contain inline text entry
// interactions. Text holds a {{response-identifier}} marker where each
// interaction sits, so a candidate view can show the blank in place.
type Paragraph struct {
	Text        string
	TextEntries []TextEntryInteraction
}

// UnmarshalXML reads a <p> element, keeping the text of any inline
// formatting such as <strong>, and replacing each text entry interaction
// with a marker in the text. encoding/xml calls it in place of decoding the
// struct from tags.
func (p *Paragraph) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var text strings.Builder
	depth := 0 // how many inline elements inside the <p> are open
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.StartElement:
			if t.Name.Local != "qti-text-entry-interaction" {
				depth++
				continue
			}
			var entry TextEntryInteraction
			if err := d.DecodeElement(&entry, &t); err != nil {
				return err
			}
			p.TextEntries = append(p.TextEntries, entry)
			fmt.Fprintf(&text, "{{%s}}", entry.ResponseIdentifier)
		case xml.EndElement:
			if depth > 0 {
				depth--
				continue
			}
			p.Text = text.String()
			return nil
		}
	}
}

// readText reads the rest of the current element, up to and including its
// end tag, and returns all of the text inside it, at every depth. The text of
// inline formatting such as <strong> is kept; the tags themselves are
// dropped. Call it from an UnmarshalXML method, after the start element.
func readText(d *xml.Decoder) (string, error) {
	var text strings.Builder
	depth := 0 // how many elements inside the current one are open
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}

		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 {
				return text.String(), nil
			}
			depth--
		}
	}
}

// InlineText is element text that may contain inline formatting, such as a
// prompt that says "<strong>not</strong>". Decoding it keeps the text of
// nested elements, which a plain string field would silently drop.
type InlineText string

// UnmarshalXML reads the element's text, including the text of any nested
// elements. encoding/xml calls it in place of decoding a plain string.
func (t *InlineText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	text, err := readText(d)
	if err != nil {
		return err
	}
	*t = InlineText(text)
	return nil
}

// ChoiceInteraction is a multiple-choice question. MaxChoices is 1 for a
// single answer and 0 for no limit.
type ChoiceInteraction struct {
	ResponseIdentifier string         `xml:"response-identifier,attr"`
	MaxChoices         int            `xml:"max-choices,attr"`
	Prompt             InlineText     `xml:"qti-prompt"`
	Choices            []SimpleChoice `xml:"qti-simple-choice"`
}

// SimpleChoice is one option in a ChoiceInteraction. Text keeps the text of
// any inline formatting inside the choice.
type SimpleChoice struct {
	Identifier string
	Text       string
}

// UnmarshalXML reads a <qti-simple-choice> element's identifier attribute and
// its text, including the text of any nested elements.
func (c *SimpleChoice) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		if a.Name.Local == "identifier" {
			c.Identifier = a.Value
		}
	}
	text, err := readText(d)
	if err != nil {
		return err
	}
	c.Text = text
	return nil
}

// TextEntryInteraction is a short typed answer.
type TextEntryInteraction struct {
	ResponseIdentifier string `xml:"response-identifier,attr"`
	ExpectedLength     int    `xml:"expected-length,attr"`
}

// ResponseProcessing names the scoring template for the item.
type ResponseProcessing struct {
	Template string `xml:"template,attr"`
}

// Parse reads one QTI item from r and validates it.
func Parse(r io.Reader) (*Item, error) {
	var item Item
	if err := xml.NewDecoder(r).Decode(&item); err != nil {
		return nil, fmt.Errorf("parse item: %w", err)
	}
	if err := item.Validate(); err != nil {
		return nil, err
	}
	return &item, nil
}

// ParseFile reads one QTI item from the file at path.
func ParseFile(path string) (*Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	item, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return item, nil
}

// LoadDir reads every .xml file in dir, in file name order, as one linear
// test form. It returns an error if dir holds no items or if two items share
// an identifier.
func LoadDir(dir string) ([]*Item, error) {
	// os.ReadDir returns entries sorted by file name.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var items []*Item
	seen := make(map[string]string) // identifier -> file name
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".xml" {
			continue
		}
		item, err := ParseFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if first, ok := seen[item.Identifier]; ok {
			return nil, fmt.Errorf("duplicate identifier %q in %s and %s", item.Identifier, first, e.Name())
		}
		seen[item.Identifier] = e.Name()
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no .xml items found in %s", dir)
	}
	return items, nil
}

// TemplateName returns the short name of the item's scoring template, such as
// "match_correct". It accepts either the full template URI or a bare name.
func (it *Item) TemplateName() string {
	var substrIndex = strings.LastIndex(it.ResponseProcessing.Template, "/")
	var substr = (it.ResponseProcessing.Template[substrIndex+1:])
	var returnString = strings.TrimSuffix(substr, ".xml")
	return returnString

}

// The scoring templates this engine supports.
const (
	templateMatchCorrect = "match_correct"
	templateMapResponse  = "map_response"
)

// Validate returns an error if the item is missing something this engine
// needs to deliver and score it.
func (it *Item) Validate() error {
	if it.Identifier == "" {
		return errors.New("item has no identifier")
	}
	if len(it.ResponseDeclarations) == 0 {
		return fmt.Errorf("item %q: no response declaration", it.Identifier)
	}
	if !it.hasInteraction() {
		return fmt.Errorf("item %q: no supported interaction", it.Identifier)
	}
	switch name := it.TemplateName(); name {
	case templateMatchCorrect, templateMapResponse:
		return nil
	default:
		return fmt.Errorf("item %q: unsupported scoring template %q", it.Identifier, name)
	}
}

// hasInteraction reports whether the body holds at least one interaction
// this engine can deliver.
func (it *Item) hasInteraction() bool {
	if len(it.Body.ChoiceInteractions) > 0 {
		return true
	}
	for _, p := range it.Body.Paragraphs {
		if len(p.TextEntries) > 0 {
			return true
		}
	}
	return false
}
