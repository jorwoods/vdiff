#!/usr/bin/env python
from collections.abc import Iterable, Sequence
from functools import lru_cache
import re
import shlex
import subprocess

from pygments import highlight
from pygments.lexers.diff import DiffLexer
from pygments.formatters import TerminalFormatter
from textual.app import App
from textual.containers import Horizontal, Vertical
from textual.message import Message
from textual.widgets import Button, Footer, Header, Label, ListItem, ListView, TextArea

# Expect the start of each string/line to be a commit hash.
COMMIT_HASH = re.compile(r"^([a-fA-F0-9]{5,40})\b")


def shell(command: Sequence[str] | str) -> str:
    if isinstance(command, str):
        command = shlex.split(command)
    result = subprocess.run(command, text=True, check=True, capture_output=True)
    return result.stdout


def get_commits(commits: Iterable[str]) -> list[str]:
    hashes = []
    if isinstance(commits, str):
        commits = commits.split()
    for commit in (commits or []):
        if not (valid := COMMIT_HASH.match(commit)):
            continue
        else:
            hashes.append(valid.group())
    return hashes

@lru_cache()
def get_patch(commit: str) -> str:
    return shell(["git", "show", commit])


class DiffList(ListView):
    def __init__(self) -> None:
        super().__init__(id="diff_list")
        self.selected_index = 0

    class Highlight(Message):
        def __init__(self, commit: str) -> None:
            self.commit = commit
            super().__init__()

    def on_list_view_highlighted(self, message: ListView.Highlighted):
        item = message.item
        if item is None:
            return
        if isinstance(item.children[0], Label):
            commit = str(item.children[0].content)
        self.post_message(self.Highlight(commit))

    def set_content(self, content:Iterable[str]) -> None:
        self.clear()
        self.extend([ListItem(Label(c)) for c in content])

class GetDiffs(Horizontal):
    def __init__(self) -> None:
        super().__init__(id="input_area")
        self.git_cmd = TextArea("git log", id="git_cmd")
        self.go = Button("Get Diffs", id="go_button")

    def compose(self):
        yield self.git_cmd
        yield self.go

    class CommandRun(Message):
        def __init__(self, value: Iterable[str]) -> None:
            self.value = value
            super().__init__()


    def on_button_pressed(self, message: Button.Pressed) -> None:
        cmd = self.git_cmd.text
        if "--pretty" not in cmd:
            cmd = f"{cmd} --pretty=%h"
        cmd_out = shell(shlex.split(cmd)).splitlines()
        commits = get_commits(cmd_out)
        self.post_message(self.CommandRun(commits))


class GitInfo(Vertical):
    def __init__(self) -> None:
        super().__init__(id="git_info")
        # self.get_diffs = GetDiffs()
        self.diff_list = DiffList()

    def compose(self):
        # yield self.get_diffs
        yield self.diff_list

class DiffStat(TextArea):
    def __init__(self, value: str = ""):
        super().__init__(text=value, read_only=True, id="diff_stat")

class DiffViewer(App):
    # CSS_PATH = "vdiff.tcss"
    BINDINGS = [
        ("^enter", "get_diffs", "Get Diffs"),
    ]
    def __init__(self):
        super().__init__()
        self.diff_stat = DiffStat()
        self.git_info = GitInfo()
        self.get_diffs = GetDiffs()

    def update_diff_stat(self, commit: str):
        print("update_diff_stat called for commit:", commit)
        patch = get_patch(commit)
        print("update_diff_stat called for patch:", patch[:50])
        highlighted_patch = highlight(patch, DiffLexer(), TerminalFormatter())
        self.diff_stat.text = highlighted_patch

    def on_diff_list_highlight(self, message: DiffList.Highlight) -> None:
        self.commit = message.commit
        self.update_diff_stat(self.commit)

    def on_get_diffs_command_run(self, message: GetDiffs.CommandRun) -> None:
        self.git_info.diff_list.set_content(message.value)

    def action_get_diffs(self) -> None:
        self.get_diffs.go.press()


    def compose(self):
        yield Header(id="header")
        yield self.get_diffs
        yield Horizontal(
                self.git_info,
                self.diff_stat,
                id="main"
            )
        yield Footer(id="footer")


def main():
    app = DiffViewer()
    app.run()


if __name__ == "__main__":
    main()
