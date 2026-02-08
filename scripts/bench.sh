#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../demos"
echo "═══ chanopt Benchmark Suite ═══"
go test -bench=. -benchmem -count=5 -timeout=120s 2>&1 | tee /tmp/chanopt_bench.txt
echo
echo "═══ Summary ═══"
grep -E '^Benchmark' /tmp/chanopt_bench.txt | \
  awk '{printf "%-35s %12s %12s\n", $1, $3" "$4, $5" "$6}' | sort
