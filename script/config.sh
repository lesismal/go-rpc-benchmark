#!/bin/bash

# What a run measures. Every BENCH_* variable below can be set for one run:
#   BENCH_CONNS=1000 BENCH_FRAMEWORKS=arpc,grpc bash script/benchmark.sh

# The frameworks, in name order, and the order their servers are started and
# their clients run in. Each is frameworks/<name>/server and
# frameworks/<name>/client, built by script/build.sh.
frameworks=(
    "arpc"
    "arpc_fib"
    "grpc"
    "grpc_fib"
    "kitex"
    "kitex_mux"
    "rpcx"
)

# The port each server listens on, which is also its default -addr. Kept
# below the Linux ephemeral range (32768-60999), and Docker runs reserve them
# (net.ipv4.ip_local_reserved_ports), so that a client socket in TIME_WAIT
# never holds the next server's port.
framework_port() {
    case "$1" in
        kitex) echo 20001 ;;
        arpc) echo 20002 ;;
        rpcx) echo 20003 ;;
        grpc) echo 20004 ;;
        arpc_fib) echo 20005 ;;
        grpc_fib) echo 20006 ;;
        kitex_mux) echo 20007 ;;
        *) return 1 ;;
    esac
}
bench_reserved_ports=20001-20007

# Connection counts, comma- or space-separated: one round of every framework
# at each. CI runs one count per job (.github/workflows/docker-benchmark.yml).
BENCH_CONNS=${BENCH_CONNS:-10,100,1000,5000,10000}
# Goroutines calling at once, the client's -c. 0 lets the client pick the
# larger of the connection count and 100: 10 calls in flight on each of 10
# connections, and one call in flight on each connection from 100 up.
BENCH_CONCURRENCY=${BENCH_CONCURRENCY:-0}
# Payload bytes every call echoes, the client's -b.
BENCH_PAYLOAD=${BENCH_PAYLOAD:-1024}
# Measured time and the unmeasured warm-up before it, the client's -d and
# -warmup.
BENCH_DURATION=${BENCH_DURATION:-10s}
BENCH_WARMUP=${BENCH_WARMUP:-5s}
# Milliseconds the server holds every call before it answers, the client's
# -sleep; 0 answers at once.
BENCH_SLEEP=${BENCH_SLEEP:-0}
# Timeout of every call and dial, the client's -timeout.
BENCH_TIMEOUT=${BENCH_TIMEOUT:-5s}
# Seconds of quiet between two runs.
BENCH_SLEEP_BETWEEN=${BENCH_SLEEP_BETWEEN:-3}
# Seconds a server has to start listening before it counts as failed.
BENCH_SERVER_START_TIMEOUT=${BENCH_SERVER_START_TIMEOUT:-30}
# The Summary's Project row: what the run benchmarks. ${VAR-default} rather
# than ${VAR:-default}, so that an empty value is kept and leaves the row out.
BENCH_PROJECT=${BENCH_PROJECT-GO-RPC-BENCHMARK}
# The order the report's Echo tables put their rows in, which carries the same
# rows and numbers either way:
#   result     (default) best first: by TPS, a tie broken by CPU EER, and that
#              by MEM EER - the columns marked [↓1], [↓2] and [↓3]
#   framework  by framework name, which puts a framework on the same row in
#              every table and every run
# bash script/report.sh -sort=framework re-reads a finished run the other way.
BENCH_REPORT_SORT=${BENCH_REPORT_SORT:-result}
case "$BENCH_REPORT_SORT" in
    result|framework) ;;
    *) echo "Unsupported BENCH_REPORT_SORT: $BENCH_REPORT_SORT (want result or framework)" >&2; return 1 ;;
esac

for v in BENCH_CONCURRENCY BENCH_PAYLOAD BENCH_SLEEP BENCH_SLEEP_BETWEEN BENCH_SERVER_START_TIMEOUT; do
    case "${!v}" in
        ''|*[!0-9]*) echo "$v must be a non-negative integer, got: ${!v}" >&2; return 1 ;;
    esac
done
bench_conns=()
IFS=', ' read -r -a requested_conns <<< "$BENCH_CONNS"
for c in "${requested_conns[@]}"; do
    [ -n "$c" ] || continue
    case "$c" in
        *[!0-9]*|0) echo "BENCH_CONNS must list positive integers, got: $BENCH_CONNS" >&2; return 1 ;;
    esac
    bench_conns+=("$c")
done
if [ "${#bench_conns[@]}" -eq 0 ]; then
    echo "BENCH_CONNS must list at least one connection count" >&2
    return 1
fi

# Optional comma-separated subset of the frameworks above.
if [ -n "${BENCH_FRAMEWORKS:-}" ]; then
    all_frameworks=("${frameworks[@]}")
    IFS=',' read -r -a requested_frameworks <<< "$BENCH_FRAMEWORKS"
    frameworks=()
    for requested in "${requested_frameworks[@]}"; do
        found=false
        for available in "${all_frameworks[@]}"; do
            if [ "$requested" = "$available" ]; then
                found=true
                break
            fi
        done
        if [ "$found" != true ]; then
            echo "Unsupported framework in BENCH_FRAMEWORKS: $requested (want some of: ${all_frameworks[*]})" >&2
            return 1
        fi
        frameworks+=("$requested")
    done
    if [ "${#frameworks[@]}" -eq 0 ]; then
        echo "BENCH_FRAMEWORKS must select at least one framework" >&2
        return 1
    fi
fi
