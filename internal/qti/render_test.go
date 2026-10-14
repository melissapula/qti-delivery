package qti

import "testing"

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
