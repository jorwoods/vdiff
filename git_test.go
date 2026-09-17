package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
			name:         "multiple commits, one per line",
			input:        "abc12 First\ndef34 Second\n0a1b2 Third",
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
			name:         "multiple stash entries",
			input:        "stash@{0}: On main: abc12 feature\nstash@{1}: On dev: def34 wip",
			wantIDs:      []string{"stash@{0}", "stash@{1}"},
			wantDisplays: []string{"stash@{0}: On main: abc12 feature", "stash@{1}: On dev: def34 wip"},
		},
		{
			name:         "blank lines between entries are ignored",
			input:        "\nabc12 First\n\ndef34 Second\n",
			wantIDs:      []string{"abc12", "def34"},
			wantDisplays: []string{"abc12 First", "def34 Second"},
		},
		{
			name:         "non-hex line with no id is skipped",
			input:        "abc12 Valid\nnot-a-hash invalid\ndef34 Also valid",
			wantIDs:      []string{"abc12", "def34"},
			wantDisplays: []string{"abc12 Valid", "def34 Also valid"},
		},
		{
			name:         "uppercase hex is accepted",
			input:        "ABC12 Uppercase hash",
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

// ── splitArgs ─────────────────────────────────────────────────────────────────

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string yields no args",
			input: "",
			want:  nil,
		},
		{
			name:  "plain unquoted command splits on whitespace",
			input: "git log --oneline -50",
			want:  []string{"git", "log", "--oneline", "-50"},
		},
		{
			name:  "tabs also separate unquoted args",
			input: "git\tlog",
			want:  []string{"git", "log"},
		},
		{
			name:  "leading and trailing whitespace produces no empty tokens",
			input: "  git log  ",
			want:  []string{"git", "log"},
		},
		{
			name:  "single quotes preserve internal whitespace as one arg",
			input: `-G 'foo bar'`,
			want:  []string{"-G", "foo bar"},
		},
		{
			name:  "double quotes preserve internal whitespace as one arg",
			input: `-G "foo bar"`,
			want:  []string{"-G", "foo bar"},
		},
		{
			name:  "quote directly attached to a flag concatenates into one token",
			input: `-G"foo bar"`,
			want:  []string{"-Gfoo bar"},
		},
		{
			name:  "unquoted prefix and suffix fuse around a quoted middle",
			input: `pre"mid dle"post`,
			want:  []string{"premid dlepost"},
		},
		{
			name:  "multiple quoted segments concatenate into a single token",
			input: `'foo'bar"baz qux"`,
			want:  []string{"foobarbaz qux"},
		},
		{
			name:  "double-quoted empty argument stands alone as an empty string",
			input: `git log -G ""`,
			want:  []string{"git", "log", "-G", ""},
		},
		{
			name:  "single-quoted empty argument stands alone as an empty string",
			input: `git log -G ''`,
			want:  []string{"git", "log", "-G", ""},
		},
		{
			name:  "empty quotes attached to unquoted text vanish without a gap",
			input: `-G""bar`,
			want:  []string{"-Gbar"},
		},
		{
			name:  "mixed quoted and unquoted args in one command",
			input: `git log -G "hello world" -- 'my file.txt'`,
			want:  []string{"git", "log", "-G", "hello world", "--", "my file.txt"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitArgs(tc.input)
			if !slices.Equal(got, tc.want) {
				t.Errorf("splitArgs(%q):\n got  %#v\n want %#v", tc.input, got, tc.want)
			}
		})
	}
}

// ── processGitCommand / getPatch (call paths through splitArgs) ────────────────

// runGit runs a git command against dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeRepoFile writes content to name inside dir.
func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// chdir switches the process working directory to dir for the duration of
// the test. processGitCommand and getPatch shell out using the process's
// cwd, so exercising their real call paths requires a real repo underfoot.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

// newTestRepo creates a temp git repo with two commits. The second commit
// adds a "hello world" line to notes.txt and a "beta line" line to a
// tracked file whose name contains a space, so quoted multi-word -G
// patterns and quoted file pathspecs both have something to bite into. It
// returns the repo dir and the second commit's full hash.
func newTestRepo(t *testing.T) (dir, headCommit string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	writeRepoFile(t, dir, "notes.txt", "line one\n")
	writeRepoFile(t, dir, "my file.txt", "alpha\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial")

	writeRepoFile(t, dir, "notes.txt", "line one\nhello world\n")
	writeRepoFile(t, dir, "my file.txt", "alpha\nbeta line\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add greeting and beta line")

	headCommit = runGit(t, dir, "rev-parse", "HEAD")
	return dir, headCommit
}

func TestProcessGitCommandQuotedPickaxePattern(t *testing.T) {
	dir, _ := newTestRepo(t)
	chdir(t, dir)

	ids, displays, _, finalCmd, err := processGitCommand(`git log -G "hello world"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want exactly 1 commit matching the quoted pickaxe pattern, got %d: %v (cmd=%q)", len(ids), displays, finalCmd)
	}

	ids, _, _, _, err = processGitCommand(`git log -G "no such phrase here"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want 0 commits matching a non-existent quoted pattern, got %d: %v", len(ids), ids)
	}
}

func TestGetPatchQuotedFileArg(t *testing.T) {
	dir, head := newTestRepo(t)
	chdir(t, dir)

	patch, err := getPatch(head, `"my file.txt"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(patch, "my file.txt") {
		t.Errorf("expected patch to reference the quoted filename, got:\n%s", patch)
	}
	if !strings.Contains(patch, "beta line") {
		t.Errorf("expected patch to contain the change to the quoted file, got:\n%s", patch)
	}
	if strings.Contains(patch, "hello world") || strings.Contains(patch, "notes.txt") {
		t.Errorf("expected patch to be scoped to the quoted filename only, got:\n%s", patch)
	}
}
