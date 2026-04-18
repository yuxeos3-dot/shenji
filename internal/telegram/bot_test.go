package telegram

import "testing"

// #41: escapeHTML must replace the three reserved characters for Telegram's
// HTML parse mode (&, <, >). Order matters — & must be handled first so we
// don't double-escape the entities emitted for < and >.
func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no_special", "hello world", "hello world"},
		{"amp_only", "a&b", "a&amp;b"},
		{"lt_only", "a<b", "a&lt;b"},
		{"gt_only", "a>b", "a&gt;b"},
		{"all_three", "<a href=\"x\">&y</a>", "&lt;a href=\"x\"&gt;&amp;y&lt;/a&gt;"},
		{"amp_first", "&<>", "&amp;&lt;&gt;"},
		{"no_double_escape", "&amp;", "&amp;amp;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeHTML(tc.in)
			if got != tc.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
