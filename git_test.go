package main

import (
	"slices"
	"testing"
)

// ── transformCmd ──────────────────────────────────────────────────────────────

func TestTransformCmd(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCmd   string
		wantFiles string
	}{
		{
			name:    "plain git log gets --oneline appended",
			input:   "git log",
			wantCmd: "git log --oneline",
		},
		{
			name:    "git log --oneline is unchanged",
			input:   "git log --oneline",
			wantCmd: "git log --oneline",
		},
		{
			name:    "existing --pretty flag suppresses --oneline",
			input:   "git log --pretty=%h",
			wantCmd: "git log --pretty=%h",
		},
		{
			name:    "existing --format flag suppresses --oneline",
			input:   "git log --format=%s",
			wantCmd: "git log --format=%s",
		},
		{
			name:    "extra flags are preserved alongside --oneline",
			input:   "git log -n 50",
			wantCmd: "git log -n 50 --oneline",
		},
		{
			name:      "file filter after -- is extracted",
			input:     "git log -- main.go",
			wantCmd:   "git log --oneline -- main.go",
			wantFiles: "main.go",
		},
		{
			name:      "multiple files after -- are extracted",
			input:     "git log -- foo.go bar.go",
			wantCmd:   "git log --oneline -- foo.go bar.go",
			wantFiles: "foo.go bar.go",
		},
		{
			name:      "--oneline + file filter round-trips correctly",
			input:     "git log --oneline -- go.mod",
			wantCmd:   "git log --oneline -- go.mod",
			wantFiles: "go.mod",
		},
		{
			name:    "stash list is passed through unchanged",
			input:   "git stash list",
			wantCmd: "git stash list",
		},
		{
			name:    "stash show is passed through unchanged",
			input:   "git stash show -p stash@{0}",
			wantCmd: "git stash show -p stash@{0}",
		},
		{
			name:    "leading and trailing whitespace is trimmed",
			input:   "  git log  ",
			wantCmd: "git log --oneline",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCmd, gotFiles := transformCmd(tc.input)
			if gotCmd != tc.wantCmd {
				t.Errorf("cmd:   got %q\n       want %q", gotCmd, tc.wantCmd)
			}
			if gotFiles != tc.wantFiles {
				t.Errorf("files: got %q\n       want %q", gotFiles, tc.wantFiles)
			}
		})
	}
}

// ── parseOutput ───────────────────────────────────────────────────────────────

func TestParseOutput(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantIDs      []string
		wantDisplays []string
	}{
		{
			name: "empty input yields nothing",
		},
		{
			name:         "single short commit hash with subject",
			input:        "abc12 Initial commit",
			wantIDs:      []string{"abc12"},
			wantDisplays: []string{"abc12 Initial commit"},
		},
		{
			name:         "full 40-char hash is accepted",
			input:        "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2 Big refactor",
			wantIDs:      []string{"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"},
			wantDisplays: []string{"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2 Big refactor"},
		},
		{
			name:         "4-char hex string is too short and skipped",
			input:        "a1b2 A commit",
			wantIDs:      nil,
			wantDisplays: nil,
		},
		{
			name:  "multiple commits, one per line",
			input: "abc12 First\ndef34 Second\n0a1b2 Third",
			wantIDs:      []string{"abc12", "def34", "0a1b2"},
			wantDisplays: []string{"abc12 First", "def34 Second", "0a1b2 Third"},
		},
		{
			name:         "stash reference is extracted",
			input:        "stash@{0}: WIP on main: abc12 message",
			wantIDs:      []string{"stash@{0}"},
			wantDisplays: []string{"stash@{0}: WIP on main: abc12 message"},
		},
		{
			name:  "multiple stash entries",
			input: "stash@{0}: On main: abc12 feature\nstash@{1}: On dev: def34 wip",
			wantIDs:      []string{"stash@{0}", "stash@{1}"},
			wantDisplays: []string{"stash@{0}: On main: abc12 feature", "stash@{1}: On dev: def34 wip"},
		},
		{
			name:  "blank lines between entries are ignored",
			input: "\nabc12 First\n\ndef34 Second\n",
			wantIDs:      []string{"abc12", "def34"},
			wantDisplays: []string{"abc12 First", "def34 Second"},
		},
		{
			name:  "non-hex line with no id is skipped",
			input: "abc12 Valid\nnot-a-hash invalid\ndef34 Also valid",
			wantIDs:      []string{"abc12", "def34"},
			wantDisplays: []string{"abc12 Valid", "def34 Also valid"},
		},
		{
			name:  "uppercase hex is accepted",
			input: "ABC12 Uppercase hash",
			wantIDs:      []string{"ABC12"},
			wantDisplays: []string{"ABC12 Uppercase hash"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotIDs, gotDisplays := parseOutput(tc.input)
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Errorf("IDs:      got %v\n          want %v", gotIDs, tc.wantIDs)
			}
			if !slices.Equal(gotDisplays, tc.wantDisplays) {
				t.Errorf("displays: got %v\n          want %v", gotDisplays, tc.wantDisplays)
			}
		})
	}
}
