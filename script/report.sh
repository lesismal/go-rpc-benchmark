#!/bin/bash

# Writes output/report/report.md from the clients' JSON reports and prints it:
# the Summary, every framework's TPS at each connection count, and an Echo
# table for each count. BENCH_PROJECT and BENCH_REPORT_SORT (script/config.sh)
# give its -project and -sort; flags given here come after them, so they win:
#
#   bash script/report.sh -sort=framework

if [ ! -x ./output/bin/report ]; then
    go build -buildvcs=false -o ./output/bin/report ./report || { return 1 2>/dev/null || exit 1; }
fi
./output/bin/report -dir ./output/report -out ./output/report/report.md \
    -project="${BENCH_PROJECT-GO-RPC-BENCHMARK}" -sort="${BENCH_REPORT_SORT:-result}" "$@"
