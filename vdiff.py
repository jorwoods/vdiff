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
from textual.widgets import ListItem, ListView, TextArea

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



class DiffList(ListView):
    ...

class DiffStat(TextArea):
    ...

class DiffViewer(App):
    def compose(self):
        yield Horizontal(
                DiffList(),
                DiffStat(),
            )


def main():
    app = DiffViewer()
    app.run()


if __name__ == "__main__":
    main()
