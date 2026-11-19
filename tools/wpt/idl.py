"""Writes the table light's prelude finishes its interfaces from.

Reads the Web IDL of standards (the pinned clone's interfaces/NAME.idl)
and prints, for each interface, what a page can observe of it without
calling anything: how many arguments each operation needs, which
members are attributes, its constants, and the members a `with` block
leaves out.

    python3 tools/wpt/idl.py dom > internal/dom/js/88-idl.js
"""

import json
import pathlib
import re
import sys

CLONE = pathlib.Path(__file__).resolve().parents[2] / ".wpt" / "wpt" / "interfaces"
SKIPPED = {"Window"}


def members(body):
    """Splits an interface body into members, at each top-level semicolon."""
    out, depth, start = [], 0, 0
    for i, char in enumerate(body):
        if char in "([{<":
            depth += 1
        elif char in ")]}>":
            depth -= 1
        elif char == ";" and depth == 0:
            out.append(" ".join(body[start:i].split()))
            start = i + 1
    return [member for member in out if member]


def arguments(text):
    """Splits an argument list at its top-level commas."""
    out, depth, start = [], 0, 0
    for i, char in enumerate(text):
        if char in "([{<":
            depth += 1
        elif char in ")]}>":
            depth -= 1
        elif char == "," and depth == 0:
            out.append(text[start:i].strip())
            start = i + 1
    return [arg for arg in out + [text[start:].strip()] if arg]


def required(text):
    count = 0
    for arg in arguments(text):
        arg = re.sub(r"^\[[^\]]*\]\s*", "", arg)
        if arg.startswith("optional ") or "..." in arg:
            break
        count += 1
    return count


def read(member, into):
    unscopable = False
    attrs = re.match(r"^\[([^\]]*)\]\s*", member)
    if attrs:
        unscopable = "Unscopable" in attrs.group(1)
        member = member[attrs.end():]
    const = re.match(r"^const .+? (\w+) = (\S+)$", member)
    if const:
        into["consts"][const.group(1)] = int(const.group(2), 0)
        return
    ctor = re.match(r"^constructor\((.*)\)$", member)
    if ctor:
        into["length"] = min(into.get("length", 99), required(ctor.group(1)))
        return
    if member == "stringifier" or member.startswith("stringifier attribute"):
        into["ops"].setdefault("toString", 0)
    if member.startswith("iterable<"):
        for name, count in (("entries", 0), ("keys", 0), ("values", 0), ("forEach", 1)):
            into["ops"].setdefault(name, count)
        return
    static = member.startswith("static ")
    member = re.sub(r"^(static|stringifier|inherit) ", "", member)
    attribute = re.match(r"^(?:readonly )?attribute .*?(\w+)$", member)
    if attribute:
        into["statics" if static else "attrs"].setdefault(attribute.group(1), 0)
        name = attribute.group(1)
    else:
        operation = re.match(r"^(?:(?:getter|setter|deleter) )?.*?(\w+)\((.*)\)$", member)
        if not operation:
            return
        name, count = operation.group(1).lstrip("_"), required(operation.group(2))
        kind = into["statics" if static else "ops"]
        kind[name] = min(kind.get(name, 99), count)
    if unscopable:
        into["unscopables"].append(name)


def table(source):
    source = re.sub(r"//[^\n]*|/\*.*?\*/", "", source, flags=re.S)
    found, includes = {}, []
    blank = lambda: {"ops": {}, "attrs": {}, "statics": {}, "consts": {}, "unscopables": []}
    pattern = r"(?:\[[^\]]*\]\s*)?(partial\s+)?(callback\s+)?interface\s+(mixin\s+)?(\w+)\s*(?::\s*\w+\s*)?\{(.*?)\n\};"
    for match in re.finditer(pattern, source, flags=re.S):
        into = found.setdefault(match.group(4), blank())
        into["mixin"] = bool(match.group(3))
        for member in members(match.group(5)):
            read(member, into)
        if match.group(2):
            into["ops"] = {}
    for match in re.finditer(r"^(\w+) includes (\w+);", source, flags=re.M):
        includes.append(match.groups())
    for name, mixin in includes:
        if name in found and mixin in found:
            for kind in ("ops", "attrs", "statics", "consts"):
                for key, value in found[mixin][kind].items():
                    found[name][kind].setdefault(key, value)
            found[name]["unscopables"] += found[mixin]["unscopables"]
    out = {}
    for name, into in sorted(found.items()):
        if into.pop("mixin") or name in SKIPPED:
            continue
        row = {key: value for key, value in into.items() if value}
        out[name] = row
    return out


def main():
    merged = {}
    for name in sys.argv[1:]:
        merged.update(table((CLONE / f"{name}.idl").read_text()))
    print("// ---- what the standards' Web IDL says of light's interfaces")
    print("//")
    print(f"// Written by tools/wpt/idl.py from: {', '.join(sys.argv[1:])}. Of each interface: the")
    print("// arguments each operation needs (ops), its attributes (attrs),")
    print("// its static members (statics), its constants (consts), the")
    print("// arguments its constructor needs (length) and the members a")
    print("// with block leaves out (unscopables).")
    print("const IDL = {")
    for name, row in merged.items():
        print(f"  {name}: {json.dumps(row, separators=(',', ':'))},")
    print("};")


if __name__ == "__main__":
    main()
