#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)
cd "$repo_root"

usage() {
    cat <<'USAGE'
Run script/benchmark.sh in a resource-limited Docker container.

Usage:
  bash script/docker_benchmark.sh [options] [benchmark client flags]

Options:
  --smoke       A short run of every framework at 10 connections.
  --rebuild     Rebuild the image without Docker's layer cache.
  -cput=N       CPUs the container gets in all (DOCKER_BENCH_CPUS)
  -cpus=N       Of those, CPUs the servers are pinned to with taskset
                (DOCKER_BENCH_SERVER_CPUS)
  -cpuc=N       Of those, CPUs the clients are pinned to with taskset
                (DOCKER_BENCH_CLIENT_CPUS)
  -h, --help    Show this help.

  The -cpu* flags may come anywhere among the arguments; the rest go to every
  benchmark client after its own flags. A -cpu* flag left out is worked out
  from the others: -cput defaults to -cpus + -cpuc when both are given, else
  to about 75% of what Docker exposes; -cpus and -cpuc default to what -cput
  leaves after the other, or to half of it each. The server and client CPUs
  never overlap.

Environment overrides (see script/config.sh for the BENCH_* ones):
  BENCH_FRAMEWORKS         Comma-separated framework subset
  BENCH_CONNS              Connection counts (default: 10,100,1000,5000,10000)
  BENCH_CONCURRENCY        Goroutines calling at once (0: the client's default)
  BENCH_PAYLOAD            Payload bytes (default: 1024)
  BENCH_DURATION           Measured time (default: 10s)
  BENCH_WARMUP             Warm-up time (default: 5s)
  BENCH_SLEEP              Milliseconds the server holds every call (default: 0)
  BENCH_TIMEOUT            Timeout of every call and dial (default: 5s)
  BENCH_PROJECT            The report Summary's Project row
  BENCH_REPORT_SORT        Report row order: result (default, best first) or
                           framework
  DOCKER_BENCH_CPUS        Integer CPU count (default: about 75% available);
                           -cput overrides it
  DOCKER_BENCH_SERVER_CPUS The servers' CPU count; -cpus overrides it
  DOCKER_BENCH_CLIENT_CPUS The clients' CPU count; -cpuc overrides it
  DOCKER_BENCH_MEMORY      Docker memory value such as 8g (default: 80%)
  DOCKER_BENCH_IMAGE       Image tag (default: go-rpc-benchmark:local)
  DOCKER_BENCH_OUTPUT      Result directory (default: output/docker/<timestamp>)
  DOCKER_BENCH_SKIP_BUILD  Set to 1 to reuse an existing image
  DOCKER_BENCH_GO_IMAGE    Base image (default: golang:1.27-bookworm)
  DOCKER_BENCH_APT_MIRROR  Replaces http://deb.debian.org in the image's apt
                           sources, e.g. https://mirrors.aliyun.com
  DOCKER_BENCH_GOPROXY     GOPROXY for the image's go mod download
  (script/docker_benchmark_cn.sh sets the last three to mainland China mirrors.)

Examples:
  bash script/docker_benchmark.sh --smoke
  BENCH_CONNS=1000 BENCH_FRAMEWORKS=arpc,grpc bash script/docker_benchmark.sh
  bash script/docker_benchmark.sh -cput=8 -cpus=4 -cpuc=4 -b=4096
USAGE
}

smoke=false
rebuild=false
cpu_total=${DOCKER_BENCH_CPUS:-}
cpu_server=${DOCKER_BENCH_SERVER_CPUS:-}
cpu_client=${DOCKER_BENCH_CLIENT_CPUS:-}
other_args=()
for arg in "$@"; do
    case "$arg" in
        -cput=*|--cput=*) cpu_total=${arg#*=} ;;
        -cpus=*|--cpus=*) cpu_server=${arg#*=} ;;
        -cpuc=*|--cpuc=*) cpu_client=${arg#*=} ;;
        *) other_args+=("$arg") ;;
    esac
done
set -- ${other_args[@]+"${other_args[@]}"}
for cpu_flag in "cput:$cpu_total" "cpus:$cpu_server" "cpuc:$cpu_client"; do
    cpu_value=${cpu_flag#*:}
    if [ -n "$cpu_value" ] && ! [[ "$cpu_value" =~ ^[1-9][0-9]*$ ]]; then
        echo "-${cpu_flag%%:*} must be a positive integer, got: $cpu_value" >&2
        exit 1
    fi
done
while [ "$#" -gt 0 ]; do
    case "$1" in
        --smoke) smoke=true; shift ;;
        --rebuild) rebuild=true; shift ;;
        -h|--help) usage; exit 0 ;;
        --) shift; break ;;
        *) break ;;
    esac
done
benchmark_args=("$@")

if [ "$smoke" = true ]; then
    export BENCH_CONNS=10
    export BENCH_DURATION=3s
    export BENCH_WARMUP=1s
    export BENCH_SLEEP_BETWEEN=1
fi
# Checks the BENCH_* values here, before an image is built for them.
. "$repo_root/script/config.sh" || exit 1

if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required" >&2
    exit 1
fi
if ! docker_info=$(docker info --format '{{.NCPU}} {{.MemTotal}}' 2>/dev/null); then
    echo "cannot connect to the Docker daemon; start Docker and check socket permissions" >&2
    exit 1
fi
read -r daemon_cpu_count daemon_memory_bytes <<< "$docker_info"

image=${DOCKER_BENCH_IMAGE:-go-rpc-benchmark:local}
build_args=(-f script/Dockerfile.benchmark -t "$image")
if [ "$rebuild" = true ]; then
    build_args=(--no-cache "${build_args[@]}")
fi
if [ -n "${DOCKER_BENCH_GO_IMAGE:-}" ]; then build_args+=(--build-arg "GO_IMAGE=$DOCKER_BENCH_GO_IMAGE"); fi
if [ -n "${DOCKER_BENCH_APT_MIRROR:-}" ]; then build_args+=(--build-arg "APT_MIRROR=$DOCKER_BENCH_APT_MIRROR"); fi
if [ -n "${DOCKER_BENCH_GOPROXY:-}" ]; then build_args+=(--build-arg "GO_PROXY=$DOCKER_BENCH_GOPROXY"); fi
if [ "${DOCKER_BENCH_SKIP_BUILD:-0}" != 1 ]; then
    echo "Building Docker benchmark image: $image"
    docker build "${build_args[@]}" .
elif ! docker image inspect "$image" >/dev/null 2>&1; then
    echo "DOCKER_BENCH_SKIP_BUILD=1, but image does not exist: $image" >&2
    exit 1
fi

# Docker Desktop may expose fewer CPUs than the host has: select from the
# daemon's effective cpuset rather than assuming 0-N.
allowed_cpu_spec=$(docker run --rm "$image" awk '/Cpus_allowed_list/ {print $2}' /proc/self/status)
expanded_cpus=()
IFS=',' read -r -a cpu_parts <<< "$allowed_cpu_spec"
for cpu_part in "${cpu_parts[@]}"; do
    if [[ "$cpu_part" =~ ^([0-9]+)-([0-9]+)$ ]]; then
        for ((cpu_id=BASH_REMATCH[1]; cpu_id<=BASH_REMATCH[2]; cpu_id++)); do
            expanded_cpus+=("$cpu_id")
        done
    elif [[ "$cpu_part" =~ ^[0-9]+$ ]]; then
        expanded_cpus+=("$cpu_part")
    else
        echo "unexpected Docker CPU list: $allowed_cpu_spec" >&2
        exit 1
    fi
done
available_cpus=${#expanded_cpus[@]}
if [ "$daemon_cpu_count" -lt "$available_cpus" ]; then
    available_cpus=$daemon_cpu_count
fi
if [ "$available_cpus" -lt 2 ]; then
    echo "Docker needs at least 2 CPUs for separate server and client CPUs" >&2
    exit 1
fi

if [ -n "$cpu_total" ]; then
    allocated_cpus=$cpu_total
elif [ -n "$cpu_server" ] && [ -n "$cpu_client" ]; then
    allocated_cpus=$((cpu_server + cpu_client))
else
    allocated_cpus=$((available_cpus * 3 / 4))
    if [ "$allocated_cpus" -lt 2 ]; then allocated_cpus=2; fi
fi
if [ "$allocated_cpus" -lt 2 ] || [ "$allocated_cpus" -gt "$available_cpus" ]; then
    echo "requested $allocated_cpus CPUs, but Docker exposes $available_cpus" >&2
    exit 1
fi
if [ -n "$cpu_server" ] && [ -n "$cpu_client" ]; then
    server_cpu_count=$cpu_server
    client_cpu_count=$cpu_client
elif [ -n "$cpu_server" ]; then
    server_cpu_count=$cpu_server
    client_cpu_count=$((allocated_cpus - cpu_server))
elif [ -n "$cpu_client" ]; then
    client_cpu_count=$cpu_client
    server_cpu_count=$((allocated_cpus - cpu_client))
else
    server_cpu_count=$((allocated_cpus / 2))
    client_cpu_count=$((allocated_cpus - server_cpu_count))
fi
if [ "$server_cpu_count" -lt 1 ] || [ "$client_cpu_count" -lt 1 ] \
    || [ $((server_cpu_count + client_cpu_count)) -gt "$allocated_cpus" ]; then
    echo "cannot pin $server_cpu_count server and $client_cpu_count client CPUs within $allocated_cpus;" >&2
    echo "each needs at least 1, and together no more than the container's (-cput)" >&2
    exit 1
fi

join_cpus() {
    local joined="" cpu
    for cpu in "$@"; do
        joined="${joined}${joined:+,}${cpu}"
    done
    printf '%s' "$joined"
}
selected_cpus=("${expanded_cpus[@]:0:allocated_cpus}")
selected_cpu_list=$(join_cpus "${selected_cpus[@]}")
server_cpu_list=$(join_cpus "${selected_cpus[@]:0:server_cpu_count}")
client_cpu_list=$(join_cpus "${selected_cpus[@]:server_cpu_count:client_cpu_count}")

if [ -n "${DOCKER_BENCH_MEMORY:-}" ]; then
    memory_limit=$DOCKER_BENCH_MEMORY
    memory_description=$DOCKER_BENCH_MEMORY
else
    memory_limit=$((daemon_memory_bytes * 80 / 100))
    memory_description=$(awk -v bytes="$memory_limit" 'BEGIN {printf "%.2f GiB", bytes/1024/1024/1024}')
fi

timestamp=$(date +%Y%m%d-%H%M%S)
result_dir=${DOCKER_BENCH_OUTPUT:-"$repo_root/output/docker/$timestamp"}
mkdir -p "$result_dir"
container="go-rpc-benchmark-${timestamp}-$$"
cleanup_container() {
    docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup_container EXIT INT TERM

run_args=(
    --name "$container"
    --init
    --network none
    --cpus "$allocated_cpus"
    --cpuset-cpus "$selected_cpu_list"
    --memory "$memory_limit"
    --memory-swap "$memory_limit"
    --pids-limit 65536
    --ulimit nofile=1048576:1048576
    # Room for the clients' ports, minus the servers', which a client socket
    # left in TIME_WAIT would otherwise hold from the next server.
    --sysctl "net.ipv4.ip_local_port_range=1024 65535"
    --sysctl "net.ipv4.ip_local_reserved_ports=$bench_reserved_ports"
    --sysctl net.ipv4.tcp_tw_reuse=1
    --sysctl net.core.somaxconn=65535
    --sysctl net.ipv4.tcp_max_syn_backlog=65535
    --env "BENCH_SERVER_CPU_LIST=$server_cpu_list"
    --env "BENCH_CLIENT_CPU_LIST=$client_cpu_list"
)
for v in BENCH_FRAMEWORKS BENCH_CONNS BENCH_CONCURRENCY BENCH_PAYLOAD BENCH_DURATION BENCH_WARMUP \
    BENCH_SLEEP BENCH_TIMEOUT BENCH_SLEEP_BETWEEN BENCH_SERVER_START_TIMEOUT BENCH_PROJECT \
    BENCH_REPORT_SORT; do
    if [ -n "${!v+set}" ]; then
        run_args+=(--env "$v=${!v}")
    fi
done

# The machine the numbers were measured on, for resources.txt.
host_cpu_model=unknown
case "$(uname -s)" in
    Darwin) host_cpu_model=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown) ;;
    Linux) host_cpu_model=$(awk -F': *' '/^model name/ {print $2; exit}' /proc/cpuinfo 2>/dev/null || echo unknown) ;;
esac
docker_os=$(docker info --format '{{.OperatingSystem}}, kernel {{.KernelVersion}}, {{.Architecture}}' 2>/dev/null || echo unknown)
cat > "$result_dir/resources.txt" <<RESOURCES
Host: $(uname -srm 2>/dev/null || echo unknown)
Host CPU model: ${host_cpu_model:-unknown}
Docker OS: $docker_os
Docker CPUs total: $daemon_cpu_count
Docker CPUs available: $available_cpus ($allowed_cpu_spec)
Docker CPUs allocated: $allocated_cpus ($selected_cpu_list)
Server CPUs: $server_cpu_count ($server_cpu_list)
Client CPUs: $client_cpu_count ($client_cpu_list)
Container memory limit: $memory_description
Frameworks: ${BENCH_FRAMEWORKS:-all}
Connections: $BENCH_CONNS
Concurrency: $BENCH_CONCURRENCY (0: the larger of the connections and 100)
Payload: $BENCH_PAYLOAD bytes
Duration: $BENCH_DURATION after $BENCH_WARMUP of warm-up
Server sleep: ${BENCH_SLEEP}ms
Client flags: ${benchmark_args[*]:-none}
RESOURCES
cat "$result_dir/resources.txt"
echo "Results: $result_dir"

set +e
docker run "${run_args[@]}" "$image" \
    bash script/benchmark.sh ${benchmark_args[@]+"${benchmark_args[@]}"} 2>&1 | tee "$result_dir/console.log"
benchmark_status=${PIPESTATUS[0]}
set -e

mkdir -p "$result_dir/report" "$result_dir/log"
docker cp "$container:/workspace/output/report/." "$result_dir/report" >/dev/null 2>&1 || true
docker cp "$container:/workspace/output/log/." "$result_dir/log" >/dev/null 2>&1 || true

if [ "$(docker inspect --format '{{.State.OOMKilled}}' "$container" 2>/dev/null)" = true ]; then
    {
        echo "The container ran out of memory: the kernel killed a server or a client at its $memory_description limit."
        echo "Give it more with DOCKER_BENCH_MEMORY (Docker exposes $daemon_memory_bytes bytes), or ask for fewer connections."
    } | tee -a "$result_dir/console.log" >&2
    if [ "$benchmark_status" -eq 0 ]; then
        benchmark_status=1
    fi
fi

if [ "$benchmark_status" -eq 0 ] && [ ! -f "$result_dir/report/report.md" ]; then
    echo "Docker benchmark produced no report" >&2
    benchmark_status=1
fi
if [ "$benchmark_status" -ne 0 ]; then
    echo "Docker benchmark failed with status $benchmark_status; partial output is in $result_dir" >&2
    exit "$benchmark_status"
fi
echo "Docker benchmark completed: $result_dir"
