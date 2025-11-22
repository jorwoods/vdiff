#!/usr/bin/env python
import argparse
from collections.abc import Iterable, Sequence
from functools import lru_cache
import re
import shlex
import subprocess
import sys

from pygments import highlight
from pygments.lexers.diff import DiffLexer
from pygments.formatters import TerminalFormatter
from textual.app import App
from textual.containers import Horizontal
from textual.message import Message
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
    def __init__(self, commits: Iterable[str], on_select=None) -> None:
        super().__init__()
        self.commits = list(commits)
        self.on_select = on_select
        self.selected_index = 0

    class Highlight(Message):
        def __init__(self, commit: str) -> None:
            self.commit = commit
            super().__init__()

    def compose(self):
        for commit in self.commits:
            yield ListItem(Label(commit))

    def on_list_view_highlighted(self, message: ListView.Highlighted):
        item = message.item
        if item is None:
            return
        if isinstance(item.children[0], Label):
            commit = str(item.children[0].content)
        self.post_message(self.Highlight(commit))


class DiffStat(TextArea):
    def __init__(self, value: str = ""):
        super().__init__(text=value, read_only=True)
    ...

class DiffViewer(App):
    def __init__(self, commits: Iterable[str]):
        super().__init__()
        self.diff_stat = DiffStat()
        self.diff_list = DiffList(commits, on_select=self.update_diff_stat)

    def update_diff_stat(self, commit: str):
        print("update_diff_stat called for commit:", commit)
        patch = get_patch(commit)
        print("update_diff_stat called for patch:", patch[:50])
        highlighted_patch = highlight(patch, DiffLexer(), TerminalFormatter())
        self.diff_stat.text = highlighted_patch

    def on_diff_list_highlight(self, message: DiffList.Highlight) -> None:
        self.commit = message.commit
        self.update_diff_stat(self.commit)


    def compose(self):
        yield Horizontal(
                self.diff_list,
                self.diff_stat,
            )


def main():
    args = parse_args()
    commits = get_commits(args.commits)
    app = DiffViewer(commits)
    app.run()


if __name__ == "__main__":
    main()
