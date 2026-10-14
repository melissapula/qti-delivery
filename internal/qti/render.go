package qti

import "strings"

// squeeze collapses each run of whitespace in s, including newlines and
// tabs, into a single space, and removes whitespace from both ends.
func squeeze(s string) string {
	words := strings.Fields(s)
	finalString := strings.Join(words, " ")
	return finalString
}
