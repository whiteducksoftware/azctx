"""Gate statement-weighted internal/ coverage; read a Go coverprofile on stdin."""

import re
import sys


INTERNAL = "github.com/whiteducksoftware/azctx/internal/"
BLOCK = re.compile(r"(\S+):([1-9]\d*)\.([1-9]\d*),([1-9]\d*)\.([1-9]\d*) (\d+) (\d+)")


def check(stream):
    if stream.readline().strip() not in ("mode: atomic", "mode: count", "mode: set"):
        raise ValueError("missing or unsupported mode")
    blocks = {}
    for number, line in enumerate(stream, 2):
        match = BLOCK.fullmatch(line.strip())
        if match is None:
            raise ValueError(f"malformed block on line {number}")
        path, start_line, start_col, end_line, end_col, statements, count = match.groups()
        key = match.groups()[:5]
        if (int(start_line), int(start_col)) > (int(end_line), int(end_col)):
            raise ValueError(f"reversed block on line {number}")
        statements, hit = int(statements), int(count) > 0
        if key in blocks:
            previous_statements, previous_hit = blocks[key]
            if statements != previous_statements:
                raise ValueError(f"inconsistent block on line {number}")
            hit = hit or previous_hit
        blocks[key] = (statements, hit)
    # -coverpkg repeats blocks for each test binary. Union hits, weight once.
    internal = [value for key, value in blocks.items() if key[0].startswith(INTERNAL)]
    total = sum(statements for statements, _ in internal)
    covered = sum(statements for statements, hit in internal if hit)
    if total == 0:
        raise ValueError("no internal statements")
    print(f"internal/: {covered}/{total} statements ({covered / total:.2%}); minimum 70%")
    return 0 if covered * 100 >= total * 70 else 1


if __name__ == "__main__":
    try:
        sys.exit(check(sys.stdin))
    except ValueError as error:
        print(f"invalid coverage: {error}", file=sys.stderr)
        sys.exit(1)
