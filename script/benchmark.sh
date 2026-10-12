#!/bin/bash

# Runs the benchmark on this machine: for each connection count in
# BENCH_CONNS, every framework in turn - its server started alone, its client
# run against it, the server stopped - and then the report. See
# script/config.sh for what it is configured with; any flags given here go to
# every client after its own, so they override them:
#
#   bash script/benchmark.sh
#   BENCH_CONNS=1000 BENCH_FRAMEWORKS=arpc,fib_arpc bash script/benchmark.sh -b=4096

. ./script/env.sh || { return 1 2>/dev/null || exit 1; }

echo "$line"
. ./script/killall.sh
rm -rf ./output/report ./output/log
. ./script/build.sh || { return 1 2>/dev/null || exit 1; }
echo "$line"

max_conns=0
for c in "${bench_conns[@]}"; do
    if [ "$c" -gt "$max_conns" ]; then max_conns=$c; fi
done
# Both ends of every connection, and room for each process's own files.
bench_raise_file_limit $((max_conns * 2 + 4096)) || { return 1 2>/dev/null || exit 1; }
print_env
echo "$line"

# Waits for the server with pid $1 to listen on port $2.
wait_listening() {
    local pid=$1 port=$2 waited=0
    while [ "$waited" -lt $((BENCH_SERVER_START_TIMEOUT * 10)) ]; do
        if ! kill -0 "$pid" 2>/dev/null; then
            return 1
        fi
        if (exec 3<>"/dev/tcp/127.0.0.1/${port}") 2>/dev/null; then
            return 0
        fi
        sleep 0.1
        waited=$((waited + 1))
    done
    return 1
}

# Stops the server with pid $1: SIGINT, then SIGKILL after 15 seconds.
stop_server() {
    local pid=$1
    kill -INT "$pid" 2>/dev/null
    for _ in $(seq 1 150); do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.1
    done
    kill -KILL "$pid" 2>/dev/null
    wait "$pid" 2>/dev/null
}

failed=()
first=true
for conns in "${bench_conns[@]}"; do
    for f in "${frameworks[@]}"; do
        if [ "$first" != true ]; then sleep "$BENCH_SLEEP_BETWEEN"; fi
        first=false
        port=$(framework_port "$f")
        name="c${conns}-${f}"
        echo "run ${f} server on cpu ${server_cpu_list:-unbound}, port ${port}"
        $limit_cpu_server "./output/bin/${f}.server" -addr=":${port}" >"./output/log/${name}.server.log" 2>&1 &
        server_pid=$!
        if ! wait_listening "$server_pid" "$port"; then
            echo "${f} server did not start; see output/log/${name}.server.log:" >&2
            tail -n 20 "./output/log/${name}.server.log" >&2
            stop_server "$server_pid"
            failed+=("${name} (server)")
            continue
        fi

        echo "run ${f} client on cpu ${client_cpu_list:-unbound}, ${conns} connections"
        $limit_cpu_client "./output/bin/${f}.client" \
            -addr="127.0.0.1:${port}" \
            -conns="${conns}" \
            -c="${BENCH_CONCURRENCY}" \
            -b="${BENCH_PAYLOAD}" \
            -d="${BENCH_DURATION}" \
            -warmup="${BENCH_WARMUP}" \
            -sleep="${BENCH_SLEEP}" \
            -timeout="${BENCH_TIMEOUT}" \
            -spid="${server_pid}" \
            -out="./output/report/${name}.json" \
            "$@" 2>&1 | tee "./output/log/${name}.client.log" | grep -v '^{'
        if [ "${PIPESTATUS[0]}" -ne 0 ]; then
            failed+=("${name}")
        fi
        stop_server "$server_pid"
        echo "$line"
    done
done

# The report covers whatever the clients wrote, failed runs included.
if ls ./output/report/*.json >/dev/null 2>&1; then
    . ./script/report.sh
fi
echo "$line"

if [ "${#failed[@]}" -gt 0 ]; then
    echo "failed: ${failed[*]}; see output/log" >&2
    return 1 2>/dev/null || exit 1
fi
