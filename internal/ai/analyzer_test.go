package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestExtractJSONWithBraceInString — the bug that broke hl365.com audit:
// AI puts JSON inside evidence_code, e.g. <div style="{width:100%}">.
// Old extractJSON counted the { inside the string and ended early.
func TestExtractJSONWithBraceInString(t *testing.T) {
	in := `{"chapters":[{"title":"第1类","issues":[{"id":1,"evidence_code":"<div style=\"width:100%;}\">bad</div>","title":"x"}]}]}`
	out := extractJSON(in)
	if out != in {
		t.Fatalf("extractJSON truncated wrong:\nwant: %s\ngot:  %s", in, out)
	}
	var w struct {
		Chapters []V6Chapter `json:"chapters"`
	}
	if err := json.Unmarshal([]byte(out), &w); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(w.Chapters) != 1 || len(w.Chapters[0].Issues) != 1 {
		t.Fatalf("unexpected: %+v", w)
	}
}

func TestExtractJSONStripsMarkdown(t *testing.T) {
	in := "```json\n{\"chapters\":[]}\n```"
	out := extractJSON(in)
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(out, "}") {
		t.Fatalf("markdown not stripped: %q", out)
	}
}

func TestExtractJSONWithEscapedQuote(t *testing.T) {
	// Escaped " should not toggle string state — this is what tripped up
	// ai.router.team outputs containing <meta content=\"...\">
	in := `{"a":"He said \"hi\" { here }","b":1}`
	out := extractJSON(in)
	if out != in {
		t.Fatalf("got %q", out)
	}
}

func TestRepairJSONTruncatedMidString(t *testing.T) {
	// Simulate a truncation inside a fix_suggestion string mid-way through
	// the 2nd issue. Repair should roll back to the end of the 1st issue.
	truncated := `{"chapters":[{"title":"t","issues":[{"id":1,"title":"ok"},{"id":2,"title":"partial","fix_suggestion":"half way through a string`
	repaired := repairJSON(truncated)
	var w struct {
		Chapters []V6Chapter `json:"chapters"`
	}
	if err := json.Unmarshal([]byte(repaired), &w); err != nil {
		t.Fatalf("repair failed: %v\nrepaired=%q", err, repaired)
	}
	if len(w.Chapters) != 1 || len(w.Chapters[0].Issues) < 1 {
		t.Fatalf("expected at least 1 issue preserved, got %+v", w)
	}
}
