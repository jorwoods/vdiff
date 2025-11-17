#!/usr/bin/env python
import argparse
from collections.abc import Sequence
from functools import lru_cache
import re
import subprocess
import sys

from textual.app import App
from textual.containers import Horizontal
from textual.widgets import ListItem, ListView, TextArea

# Expect the start of each string/line to be a commit hash.
COMMIT_HASH = re.compile(r"^[a-fA-F0-9]{5,40}\b")

def parse_args(args: Sequence[str] | None = None) -> argparse.Namespace:
    if args is None:
        args = sys.argv[1:]

    parser = argparse.ArgumentParser()
    parser.add_argument("--commits", "-c", nargs="+", help="Git commits to display. Pass '-' to read from stdin.")

    return parser.parse_args(args)


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
