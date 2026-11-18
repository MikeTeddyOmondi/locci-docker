#!/bin/sh
# Run govulncheck and fail on any reachable vulnerability that isn't listed,
# with its reason, in .govulncheck-ignore. Needs jq.
set -eu
cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... >"$tmp/report.json"

# Findings whose trace reaches a function are the ones the code can call.
jq -r 'select(.finding) | select(.finding.trace[0].function != null) | .finding.osv' \
	"$tmp/report.json" | sort -u >"$tmp/found"
grep -v '^#' .govulncheck-ignore | grep . | sort -u >"$tmp/ignored" || true

comm -23 "$tmp/found" "$tmp/ignored" >"$tmp/new"
if [ -s "$tmp/new" ]; then
	echo "Reachable vulnerabilities (see https://pkg.go.dev/vuln/<ID>):"
	cat "$tmp/new"
	exit 1
fi
echo "govulncheck: nothing reachable beyond .govulncheck-ignore ($(wc -l <"$tmp/found" | tr -d ' ') ignored)"
