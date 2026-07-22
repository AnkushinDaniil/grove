package github

import (
	"reflect"
	"testing"
)

func TestParsePatchEmpty(t *testing.T) {
	if got := parsePatch(""); got != nil {
		t.Errorf("parsePatch(\"\") = %+v, want nil", got)
	}
}

func TestParsePatchSingleHunk(t *testing.T) {
	patch := "@@ -1,3 +1,4 @@\n context\n-old line\n+new line\n+added\n unchanged"
	got := parsePatch(patch)
	want := []Hunk{{
		Header: "@@ -1,3 +1,4 @@",
		Lines: []DiffLine{
			{Op: " ", OldLine: 1, NewLine: 1, Text: "context"},
			{Op: "-", OldLine: 2, NewLine: 0, Text: "old line"},
			{Op: "+", OldLine: 0, NewLine: 2, Text: "new line"},
			{Op: "+", OldLine: 0, NewLine: 3, Text: "added"},
			{Op: " ", OldLine: 3, NewLine: 4, Text: "unchanged"},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePatch() =\n %+v\nwant\n %+v", got, want)
	}
}

func TestParsePatchMultiHunk(t *testing.T) {
	patch := "@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -10,2 +10,3 @@ func foo() {\n ctx\n+extra\n more"
	got := parsePatch(patch)
	if len(got) != 2 {
		t.Fatalf("hunks = %d, want 2", len(got))
	}
	if got[1].Header != "@@ -10,2 +10,3 @@ func foo() {" {
		t.Errorf("second header = %q, want the section heading preserved", got[1].Header)
	}
	// Second hunk's line counters start from the new header (old 10, new 10).
	second := got[1].Lines
	want := []DiffLine{
		{Op: " ", OldLine: 10, NewLine: 10, Text: "ctx"},
		{Op: "+", OldLine: 0, NewLine: 11, Text: "extra"},
		{Op: " ", OldLine: 11, NewLine: 12, Text: "more"},
	}
	if !reflect.DeepEqual(second, want) {
		t.Errorf("second hunk lines =\n %+v\nwant\n %+v", second, want)
	}
}

func TestParsePatchAddedFile(t *testing.T) {
	// A new file: old side is empty (@@ -0,0 +1,N @@), every line added.
	patch := "@@ -0,0 +1,2 @@\n+first\n+second"
	got := parsePatch(patch)
	if len(got) != 1 {
		t.Fatalf("hunks = %d, want 1", len(got))
	}
	want := []DiffLine{
		{Op: "+", OldLine: 0, NewLine: 1, Text: "first"},
		{Op: "+", OldLine: 0, NewLine: 2, Text: "second"},
	}
	if !reflect.DeepEqual(got[0].Lines, want) {
		t.Errorf("added-file lines =\n %+v\nwant\n %+v", got[0].Lines, want)
	}
}

func TestParsePatchRemovedFile(t *testing.T) {
	// A deleted file: new side is empty (@@ -1,2 +0,0 @@), every line removed.
	patch := "@@ -1,2 +0,0 @@\n-gone one\n-gone two"
	got := parsePatch(patch)
	if len(got) != 1 {
		t.Fatalf("hunks = %d, want 1", len(got))
	}
	want := []DiffLine{
		{Op: "-", OldLine: 1, NewLine: 0, Text: "gone one"},
		{Op: "-", OldLine: 2, NewLine: 0, Text: "gone two"},
	}
	if !reflect.DeepEqual(got[0].Lines, want) {
		t.Errorf("removed-file lines =\n %+v\nwant\n %+v", got[0].Lines, want)
	}
}

func TestParsePatchNoNewlineMarker(t *testing.T) {
	// The "\ No newline at end of file" marker is dropped, not counted.
	patch := "@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file"
	got := parsePatch(patch)
	if len(got) != 1 {
		t.Fatalf("hunks = %d, want 1", len(got))
	}
	want := []DiffLine{
		{Op: "-", OldLine: 1, NewLine: 0, Text: "old"},
		{Op: "+", OldLine: 0, NewLine: 1, Text: "new"},
	}
	if !reflect.DeepEqual(got[0].Lines, want) {
		t.Errorf("no-newline lines =\n %+v\nwant\n %+v", got[0].Lines, want)
	}
}

func TestParsePatchShortHeader(t *testing.T) {
	// A header without explicit counts (`@@ -a +b @@`) means a single line.
	got := parsePatch("@@ -5 +7 @@\n-x\n+y")
	if len(got) != 1 {
		t.Fatalf("hunks = %d, want 1", len(got))
	}
	want := []DiffLine{
		{Op: "-", OldLine: 5, NewLine: 0, Text: "x"},
		{Op: "+", OldLine: 0, NewLine: 7, Text: "y"},
	}
	if !reflect.DeepEqual(got[0].Lines, want) {
		t.Errorf("short-header lines =\n %+v\nwant\n %+v", got[0].Lines, want)
	}
}

func TestParsePatchBlankContextLine(t *testing.T) {
	// A blank context line arrives as a lone space; a trailing newline leaves an
	// empty final element that must be dropped, not treated as a blank line.
	patch := "@@ -1,3 +1,3 @@\n a\n \n b\n"
	got := parsePatch(patch)
	if len(got) != 1 {
		t.Fatalf("hunks = %d, want 1", len(got))
	}
	want := []DiffLine{
		{Op: " ", OldLine: 1, NewLine: 1, Text: "a"},
		{Op: " ", OldLine: 2, NewLine: 2, Text: ""},
		{Op: " ", OldLine: 3, NewLine: 3, Text: "b"},
	}
	if !reflect.DeepEqual(got[0].Lines, want) {
		t.Errorf("blank-context lines =\n %+v\nwant\n %+v", got[0].Lines, want)
	}
}

func TestParsePatchIgnoresPreamble(t *testing.T) {
	// Lines before the first valid hunk header are ignored.
	got := parsePatch("garbage before\n@@ -1 +1 @@\n+ok")
	if len(got) != 1 || len(got[0].Lines) != 1 || got[0].Lines[0].Text != "ok" {
		t.Errorf("parsePatch preamble handling = %+v, want a single hunk with one line", got)
	}
}
