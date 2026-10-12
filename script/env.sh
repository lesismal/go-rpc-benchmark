#!/bin/bash

# Sourced by the other scripts, from the repository root: the configuration,
# the CPUs the servers and clients are pinned to, and the file limit.

. ./script/config.sh || return 1

line=$(printf "%0.s-" {1..62})

# The servers and the clients each get half of the CPUs, by taskset, so that
# neither takes the other's: BENCH_SERVER_CPU_LIST and BENCH_CLIENT_CPU_LIST
# (taskset lists, e.g. 0-3 and 4-7) say which, and script/docker_benchmark.sh
# sets them from the container's cpuset. Without them the first half of the
# CPUs goes to the servers. Go sizes GOMAXPROCS to the CPUs a process is
# pinned to, so each side runs as many Ps as it has CPUs.
#
# Without taskset (macOS) nothing is pinned and the two share the machine.
limit_cpu_server=""
limit_cpu_client=""
server_cpu_list=""
client_cpu_list=""
if command -v taskset >/dev/null 2>&1; then
    if [ -n "${BENCH_SERVER_CPU_LIST:-}" ] && [ -n "${BENCH_CLIENT_CPU_LIST:-}" ]; then
        server_cpu_list=$BENCH_SERVER_CPU_LIST
        client_cpu_list=$BENCH_CLIENT_CPU_LIST
    else
        total_cpus=$(getconf _NPROCESSORS_ONLN)
        if [ "$total_cpus" -ge 2 ]; then
            half=$((total_cpus / 2))
            server_cpu_list="0-$((half - 1))"
            client_cpu_list="${half}-$((total_cpus - 1))"
        fi
    fi
    if [ -n "$server_cpu_list" ]; then
        limit_cpu_server="taskset -c ${server_cpu_list}"
        limit_cpu_client="taskset -c ${client_cpu_list}"
    fi
fi

# Every connection is a file on both ends, and both ends run here.
bench_raise_file_limit() {
    local want=$1
    local soft hard
    soft=$(ulimit -n)
    hard=$(ulimit -Hn)
    if [ "$soft" != unlimited ] && [ "$soft" -lt "$want" ]; then
        if [ "$hard" = unlimited ] || [ "$hard" -ge "$want" ]; then
            ulimit -n "$want" 2>/dev/null || ulimit -n "$hard" 2>/dev/null || true
        else
            ulimit -n "$hard" 2>/dev/null || true
        fi
    fi
    soft=$(ulimit -n)
    if [ "$soft" != unlimited ] && [ "$soft" -lt "$want" ]; then
        echo "the open file limit is $soft, and this run needs about $want;" >&2
        echo "raise it (ulimit -n, or the hard limit in /etc/security/limits.conf) or ask for fewer connections" >&2
        return 1
    fi
}

print_env() {
    echo "frameworks:  ${frameworks[*]}"
    echo "connections: ${bench_conns[*]}"
    echo "concurrency: ${BENCH_CONCURRENCY} (0: the larger of the connections and 100)"
    echo "payload:     ${BENCH_PAYLOAD} bytes"
    echo "duration:    ${BENCH_DURATION}, after ${BENCH_WARMUP} of warm-up"
    echo "sleep:       ${BENCH_SLEEP}ms"
    echo "server CPUs: ${server_cpu_list:-not pinned}"
    echo "client CPUs: ${client_cpu_list:-not pinned}"
    echo "open files:  $(ulimit -n)"
    go version
}
