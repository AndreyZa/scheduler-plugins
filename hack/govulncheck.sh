#!/usr/bin/env bash
set -euo pipefail

report=$(mktemp)
trap 'rm -f "$report"' EXIT
set +e
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -format json ./... > "$report"
rc=$?
set -e
if [ "$rc" -ne 0 ] && [ "$rc" -ne 3 ]; then
    exit "$rc"
fi
python3 - "$report" <<'PY'
import json
import sys

excluded = {
    "GO-2025-3521": "kubelet GitRepo volume, не код scheduler и не kubelet в образе",
    "GO-2025-3547": "kube-apiserver namespace deletion, не код scheduler и не apiserver в образе",
}
text = open(sys.argv[1]).read()
decoder = json.JSONDecoder()
pos = 0
findings = set()
while pos < len(text):
    while pos < len(text) and text[pos].isspace():
        pos += 1
    if pos >= len(text):
        break
    obj, pos = decoder.raw_decode(text, pos)
    if "finding" in obj:
        findings.add(obj["finding"]["osv"])
blocked = sorted(findings - excluded.keys())
if blocked:
    print("govulncheck: уязвимости вне списка исключений: " + ", ".join(blocked), file=sys.stderr)
    raise SystemExit(1)
for vuln in sorted(findings & excluded.keys()):
    print(f"govulncheck: исключено {vuln} — {excluded[vuln]}")
if not findings:
    print("govulncheck: уязвимостей не найдено")
PY
