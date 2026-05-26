package main

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces lipgloss to emit ANSI colour codes even when stdout has no
// TTY (the default in most CI / test environments). Without this, highlighted
// and plain lines are indistinguishable as strings and colour-sensitive tests
// would produce false negatives.
//
// SetColorProfile (rather than NewRenderer+SetDefaultRenderer) is the correct
// approach: it sets explicitColorProfile=true on the renderer so ColorProfile()
// returns TrueColor directly instead of calling output.EnvColorProfile() which
// would auto-detect the environment and strip colours.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// stripANSI removes ANSI escape sequences so rendered output can be compared
// as plain text without caring about terminal colour codes.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// ── findMatches ───────────────────────────────────────────────────────────────

func TestFindMatches(t *testing.T) {
	lines := []string{
		"abc12 Add feature",      // 0
		"def34 Fix bug in auth",  // 1
		"0a1b2 Add unit tests",   // 2
		"deadb Refactor DB code", // 3
	}

	tests := []struct {
		name string
		term string
		want []int
	}{
		{"empty term returns nil", "", nil},
		{"term not present returns nil", "zzz", nil},
		{"single exact match", "Fix bug in auth", []int{1}},
		{"case insensitive match", "FIX BUG", []int{1}},
		{"partial match on multiple lines", "Add", []int{0, 2}},
		{"hash prefix match", "abc", []int{0}},
		{"match on every line", "a", []int{0, 1, 2, 3}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findMatches(lines, tc.term)
			if !slices.Equal(got, tc.want) {
				t.Errorf("findMatches(%q) = %v, want %v", tc.term, got, tc.want)
			}
		})
	}
}

// ── visualRange ───────────────────────────────────────────────────────────────

func TestVisualRange(t *testing.T) {
	tests := []struct {
		name   string
		anchor int
		cursor int
		nLines int
		wantLo int
		wantHi int
	}{
		{"cursor below anchor", 2, 5, 10, 2, 5},
		{"cursor above anchor (reversed)", 5, 2, 10, 2, 5},
		{"cursor equals anchor (single line)", 3, 3, 10, 3, 3},
		{"cursor at last line", 0, 9, 10, 0, 9},
		{"cursor past end clamps to last line", 2, 15, 10, 2, 9},
		{"anchor at zero", 0, 4, 10, 0, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lo, hi := visualRange(tc.anchor, tc.cursor, tc.nLines)
			if lo != tc.wantLo || hi != tc.wantHi {
				t.Errorf("visualRange(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tc.anchor, tc.cursor, tc.nLines, lo, hi, tc.wantLo, tc.wantHi)
			}
		})
	}
}

// ── renderDiff ────────────────────────────────────────────────────────────────

var sampleDiff = []string{
	"commit abc12def",
	"Author: Jordan <jordan@example.com>",
	"Date:   Mon Jan 1 00:00:00 2024",
	"",
	"    Add feature",
	"",
	"diff --git a/foo.go b/foo.go",
	"index 000..111 100644",
	"--- a/foo.go",
	"+++ b/foo.go",
	"@@ -1,3 +1,4 @@",
	" context line",
	"-removed line",
	"+added line",
	" another context",
}

func TestRenderDiff_AllLinesPresent(t *testing.T) {
	output := renderDiff(sampleDiff, "", nil, 0, false, 0, 0)
	plain := stripANSI(output)

	for _, line := range sampleDiff {
		if !strings.Contains(plain, line) {
			t.Errorf("expected output to contain %q", line)
		}
	}
}

func TestRenderDiff_LineCount(t *testing.T) {
	lines := []string{"line one", "line two", "line three"}
	output := renderDiff(lines, "", nil, 0, false, 0, 0)
	// Each input line is followed by \n, so splitting produces len+1 parts.
	parts := strings.Split(output, "\n")
	want := len(lines) + 1
	if len(parts) != want {
		t.Errorf("got %d parts after split, want %d", len(parts), want)
	}
}

func TestRenderDiff_SearchMatchRenderedDifferently(t *testing.T) {
	matches := findMatches(sampleDiff, "added line")
	if len(matches) == 0 {
		t.Fatal("expected at least one match for 'added line'")
	}

	withMatch := renderDiff(sampleDiff, "added line", matches, 0, false, 0, 0)
	withoutMatch := renderDiff(sampleDiff, "", nil, 0, false, 0, 0)

	matchedLineIdx := matches[0]
	lineWith := strings.Split(withMatch, "\n")[matchedLineIdx]
	lineWithout := strings.Split(withoutMatch, "\n")[matchedLineIdx]

	if lineWith == lineWithout {
		t.Error("expected matched line to render differently from unmatched line")
	}
	// The plain text content of the matched line should still be present.
	if !strings.Contains(stripANSI(lineWith), "added line") {
		t.Errorf("matched line should still contain the original text, got %q", stripANSI(lineWith))
	}
}

func TestRenderDiff_VisualSelectionRenderedDifferently(t *testing.T) {
	anchor, cursor := 2, 5
	withVisual := renderDiff(sampleDiff, "", nil, 0, true, anchor, cursor)
	withoutVisual := renderDiff(sampleDiff, "", nil, 0, false, 0, 0)

	for i := anchor; i <= cursor; i++ {
		lineWith := strings.Split(withVisual, "\n")[i]
		lineWithout := strings.Split(withoutVisual, "\n")[i]
		if lineWith == lineWithout {
			t.Errorf("line %d should render differently inside visual selection", i)
		}
	}
}

func TestRenderDiff_LinesOutsideVisualSelectionUnchanged(t *testing.T) {
	anchor, cursor := 3, 5
	withVisual := renderDiff(sampleDiff, "", nil, 0, true, anchor, cursor)
	withoutVisual := renderDiff(sampleDiff, "", nil, 0, false, 0, 0)

	for i, line := range strings.Split(withVisual, "\n") {
		if i >= anchor && i <= cursor {
			continue // skip lines that are expected to differ
		}
		if line != strings.Split(withoutVisual, "\n")[i] {
			t.Errorf("line %d outside selection should be unchanged", i)
		}
	}
}

func TestRenderDiff_EmptyInput(t *testing.T) {
	// Should not panic and should return a single newline (empty trailing line).
	output := renderDiff([]string{}, "", nil, 0, false, 0, 0)
	if output != "" {
		t.Errorf("expected empty output for empty input, got %q", output)
	}
}
