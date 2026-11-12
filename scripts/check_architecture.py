#!/usr/bin/env python3
import argparse
import pathlib
import re
import sys


SINGLE_IMPORT = re.compile(r'^\s*import\s+"([^"\n]+)"', re.MULTILINE)
IMPORT_BLOCK = re.compile(r'^\s*import\s*\((.*?)^\s*\)', re.MULTILINE | re.DOTALL)
QUOTED_IMPORT = re.compile(r'"([^"\n]+)"')
PROJECT_MARKER = "/internal/"


def imports(path):
    text = path.read_text()
    values = [match.group(1) for match in SINGLE_IMPORT.finditer(text)]
    for block in IMPORT_BLOCK.finditer(text):
        values.extend(match.group(1) for match in QUOTED_IMPORT.finditer(block.group(1)))
    yield from (value for value in values if PROJECT_MARKER in value)


def find_violations(root):
    violations = []
    production = root / "internal"
    if not production.is_dir():
        return ["internal: production package tree is missing"]
    for path in sorted(production.rglob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        relative = path.relative_to(root).as_posix()
        for dependency in imports(path):
            if relative.startswith("internal/domain/") and any(
                marker in dependency
                for marker in (
                    "/internal/application",
                    "/internal/adapters",
                    "/internal/host",
                )
            ):
                violations.append(f"{relative}: domain imports outward dependency {dependency}")
            if relative.startswith("internal/application/") and any(
                marker in dependency
                for marker in ("/internal/adapters", "/internal/host")
            ):
                violations.append(f"{relative}: application imports outward dependency {dependency}")
    return violations


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=pathlib.Path, default=pathlib.Path(__file__).resolve().parent.parent)
    args = parser.parse_args(argv)
    violations = find_violations(args.root.resolve())
    if violations:
        print("Architecture boundary violations:", file=sys.stderr)
        for violation in violations:
            print(f"- {violation}", file=sys.stderr)
        return 1
    print("Architecture boundaries: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
