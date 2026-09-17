package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

var (
	commitRe = regexp.MustCompile(`^([a-fA-F0-9]{5,40})\b`)
	stashRe  = regexp.MustCompile(`^(stash@\{\d+\})`)
)

var (
	cacheMu sync.RWMutex
	cache   = map[string]string{}
)

// splitArgs tokenizes a command string into arguments, honoring single and
// double quotes so that patterns with spaces (e.g. -G "foo bar") are passed
// through as one argument rather than being torn apart on whitespace.
func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	hasCur := false
	quote := rune(0)

	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			hasCur = true
		case r == ' ' || r == '\t':
			if hasCur {
				args = append(args, cur.String())
				cur.Reset()
				hasCur = false
			}
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	if hasCur {
		args = append(args, cur.String())
	}
	return args
}

func shell(args []string) (string, error) {
	var filtered []string
	for _, a := range args {
		if a != "" {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("empty command")
	}
	out, err := exec.Command(filtered[0], filtered[1:]...).Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			if stderr := strings.TrimSpace(string(e.Stderr)); stderr != "" {
				return "", fmt.Errorf("%s", stderr)
			}
		}
		return "", err
	}
	return string(out), nil
}

// transformCmd normalises a user-supplied git command: it extracts any file
// filter after "-- " and adds "--oneline" when no output format is specified.
// Stash commands are returned unchanged. Both return values are trimmed.
func transformCmd(cmd string) (finalCmd, files string) {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "git stash") {
		return cmd, ""
	}
	if idx := strings.Index(cmd, "-- "); idx != -1 {
		files = strings.TrimSpace(cmd[idx+3:])
		cmd = strings.TrimSpace(cmd[:idx])
	}
	if !strings.Contains(cmd, "--pretty") && !strings.Contains(cmd, "--oneline") && !strings.Contains(cmd, "--format") {
		cmd += " --oneline"
	}
	if files != "" {
		cmd += " -- " + files
	}
	return cmd, files
}

// parseOutput extracts commit hashes / stash refs and their display strings
// from the raw text output of a git command.
func parseOutput(output string) (ids, displays []string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var id string
		if m := commitRe.FindString(line); m != "" {
			id = m
		} else if m := stashRe.FindString(line); m != "" {
			id = m
		}
		if id != "" {
			ids = append(ids, id)
			displays = append(displays, line)
		}
	}
	return
}

// processGitCommand normalises a git log/stash command, runs it, and returns
// each line's ID (hash or stash ref) alongside the full display string.
func processGitCommand(cmd string) (ids, displays []string, files, finalCmd string, err error) {
	finalCmd, files = transformCmd(cmd)
	out, err := shell(splitArgs(finalCmd))
	if err != nil {
		return nil, nil, "", "", err
	}
	ids, displays = parseOutput(out)
	return ids, displays, files, finalCmd, nil
}

func getPatch(commit, files string) (string, error) {
	key := commit + "\x00" + files
	cacheMu.RLock()
	if v, ok := cache[key]; ok {
		cacheMu.RUnlock()
		return v, nil
	}
	cacheMu.RUnlock()

	var args []string
	switch {
	case commitRe.MatchString(commit):
		args = []string{"git", "show", commit}
		if files != "" {
			args = append(args, "--")
			args = append(args, splitArgs(files)...)
		}
	case stashRe.MatchString(commit):
		args = []string{"git", "stash", "show", "-p", commit}
		if files != "" {
			args = append(args, "--")
			args = append(args, splitArgs(files)...)
		}
	default:
		return "", fmt.Errorf("%s: not a valid commit or stash ID", commit)
	}

	result, err := shell(args)
	if err != nil {
		return "", err
	}

	cacheMu.Lock()
	cache[key] = result
	cacheMu.Unlock()
	return result, nil
}
