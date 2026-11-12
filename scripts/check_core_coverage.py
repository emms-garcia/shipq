#!/usr/bin/env python3
import argparse
import dataclasses
import os
import pathlib
import subprocess
import sys


@dataclasses.dataclass(frozen=True)
class Coverage:
    covered: int
    total: int

    @property
    def percent(self):
        return self.covered * 100.0 / self.total


def read_profile(path):
    blocks = {}
    lines = path.read_text().splitlines()
    if not lines or not lines[0].startswith("mode: "):
        raise ValueError(f"{path}: missing Go coverage mode")
    for line in lines[1:]:
        fields = line.rsplit(maxsplit=2)
        if len(fields) != 3:
            raise ValueError(f"{path}: malformed coverage record")
        location = fields[0]
        statements = int(fields[1])
        count = int(fields[2])
        if statements < 0 or count < 0:
            raise ValueError(f"{path}: negative coverage record")
        previous = blocks.get(location)
        if previous is not None and previous[0] != statements:
            raise ValueError(f"{path}: inconsistent duplicate coverage block")
        blocks[location] = (statements, count > 0 or (previous is not None and previous[1]))
    total = sum(statements for statements, _ in blocks.values())
    covered = sum(statements for statements, hit in blocks.values() if hit)
    if total == 0:
        raise ValueError(f"{path}: group has no executable statements")
    return Coverage(covered=covered, total=total)


def generate(root, name, pattern, profile):
    profile.parent.mkdir(parents=True, exist_ok=True)
    environment = os.environ.copy()
    environment["GOTOOLCHAIN"] = "local"
    command = [
        "go",
        "test",
        "-count=1",
        "-race",
        "-covermode=atomic",
        f"-coverpkg={pattern}",
        f"-coverprofile={profile}",
        pattern,
    ]
    print(f"Generating {name} coverage…", flush=True)
    subprocess.run(command, cwd=root, env=environment, check=True)


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=pathlib.Path, default=pathlib.Path(__file__).resolve().parent.parent)
    parser.add_argument("--threshold", type=float, default=90.0)
    parser.add_argument("--profile", action="append", default=[], metavar="NAME=PATH")
    args = parser.parse_args(argv)
    root = args.root.resolve()
    profiles = {}
    for item in args.profile:
        name, separator, value = item.partition("=")
        if not separator or name not in {"domain", "application"}:
            parser.error("--profile must be domain=PATH or application=PATH")
        profiles[name] = pathlib.Path(value).resolve()
    groups = {
        "domain": "./internal/domain/...",
        "application": "./internal/application/...",
    }
    failed = False
    for name, pattern in groups.items():
        profile = profiles.get(name, root / "build" / "coverage" / f"core-{name}.out")
        if name not in profiles:
            generate(root, name, pattern, profile)
        try:
            coverage = read_profile(profile)
        except (OSError, ValueError) as error:
            print(error, file=sys.stderr)
            failed = True
            continue
        print(f"{name}: {coverage.percent:.1f}% ({coverage.covered}/{coverage.total} statements)")
        if coverage.percent + 1e-9 < args.threshold:
            print(f"{name}: below required {args.threshold:.1f}%", file=sys.stderr)
            failed = True
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
