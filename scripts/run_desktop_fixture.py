#!/usr/bin/env python3
import json
import os
import pathlib
import shutil
import sys


ROOT = pathlib.Path(__file__).resolve().parent.parent
HANDOFF = ROOT / "build" / "fixture" / "connection.json"


def main():
    try:
        value = json.loads(HANDOFF.read_text())
        socket_path = value["socket_path"]
    except (OSError, KeyError, json.JSONDecodeError):
        raise SystemExit("Start `make fixture-host` in another terminal first.")
    if not isinstance(socket_path, str) or not socket_path.startswith("/"):
        raise SystemExit("Fixture handoff contains an invalid socket path.")
    flutter = shutil.which("flutter")
    if flutter is None:
        raise SystemExit("flutter is not available on PATH")
    os.chdir(ROOT / "apps" / "desktop")
    os.execv(
        flutter,
        [flutter, "run", "-d", "macos", "--no-pub", f"--dart-define=SHIPQ_SOCKET_PATH={socket_path}"],
    )


if __name__ == "__main__":
    main()
