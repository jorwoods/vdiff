#!/usr/bin/env python
import argparse
from collections.abc import Iterable, Sequence
from functools import lru_cache
import re
import shlex
import subprocess
import sys

from textual.app import App
from textual.containers import Horizontal
from textual.widgets import Label, ListItem, ListView, TextArea

# Expect the start of each string/line to be a commit hash.
COMMIT_HASH = re.compile(r"^([a-fA-F0-9]{5,40})\b")

def parse_args(args: Sequence[str] | None = None) -> argparse.Namespace:
    if args is None:
        args = sys.argv[1:]

    parser = argparse.ArgumentParser()
    parser.add_argument("--commits", "-c", nargs="+", help="Git commits to display. Pass '-' to read from stdin.")

    return parser.parse_args(args)


def shell(command: Sequence[str] | str) -> str:
    if isinstance(command, str):
        command = shlex.split(command)
    result = subprocess.run(command, text=True, check=True, capture_output=True)
    return result.stdout


def get_commits(commits: Iterable[str]) -> list[str]:
    hashes = []
    i = -1
    if isinstance(commits, str):
        commits = commits.split()
    for i, commit in enumerate(commits or []):
        if commit == "-" or commit == "'-'":
            hashes += get_commits(sys.stdin.readlines())
        if not (valid := COMMIT_HASH.match(commit)):
            continue
        else:
            hashes.append(valid.group())

    if i == -1:
        hashes = shell("git log --pretty=%h").split()

    return hashes

@lru_cache()
def get_patch(commit: str) -> str:
    return shell(["git", "show", commit])



class DiffList(ListView):
    def __init__(self, commits: Iterable[str]) -> None:
        super().__init__()
        self.commits = commits

    def compose(self):
        for commit in self.commits:
            yield ListItem(Label(commit))

class DiffStat(TextArea):
    ...

class DiffViewer(App):
    def __init__(self, commits: Iterable[str]):
        super().__init__()
        self.commits = commits

    def compose(self):
        yield Horizontal(
                DiffList(self.commits),
                DiffStat(),
            )


def main():
    args = parse_args()
    commits = get_commits(args.commits)
    app = DiffViewer(commits)
    app.run()


if __name__ == "__main__":
    main()
