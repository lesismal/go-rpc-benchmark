#!/bin/bash

# Writes output/report/report.md from the clients' JSON reports and prints it.
# Flags go to the report command: bash script/report.sh -title="nightly".

if [ ! -x ./output/bin/report ]; then
    go build -buildvcs=false -o ./output/bin/report ./report || { return 1 2>/dev/null || exit 1; }
fi
./output/bin/report -dir ./output/report -out ./output/report/report.md -title="${BENCH_PROJECT-GO-RPC-BENCHMARK}" "$@"
