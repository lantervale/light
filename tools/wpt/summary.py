"""A wptreport.json as one line a test: its status, its subtests passed over all, why it failed."""

import json
import sys
from collections import Counter


def main(path):
    with open(path) as f:
        results = sorted(json.load(f)["results"], key=lambda result: result["test"])
    statuses = Counter()
    passed = total = 0
    for result in results:
        subtests = result["subtests"]
        ok = sum(1 for subtest in subtests if subtest["status"] == "PASS")
        passed += ok
        total += len(subtests)
        statuses[result["status"]] += 1
        why = ""
        if result["status"] not in ("OK", "PASS"):
            why = "  " + (result.get("message") or "").split("\n")[0][:120]
        print(f"{result['status']:<8} {ok:>5}/{len(subtests):<5} {result['test']}{why}")
    tests = ", ".join(f"{count} {status}" for status, count in sorted(statuses.items()))
    print(f"\n{len(results)} tests: {tests}; {passed}/{total} subtests pass")


if __name__ == "__main__":
    main(sys.argv[1])
