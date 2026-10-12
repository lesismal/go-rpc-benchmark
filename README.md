# go-rpc-benchmark

Benchmarks of Go RPC frameworks, all of them encoding protobuf, at 10, 100,
1000, 5000 and 10000 connections.

| Name | Framework | Protocol |
| --- | --- | --- |
| `arpc` | [github.com/lesismal/arpc](https://github.com/lesismal/arpc) | arpc |
| `arpc_fib` | [github.com/lesismal/fib/arpc](https://github.com/lesismal/fib/tree/main/arpc) | arpc, on fib's engine |
| `grpc` | [google.golang.org/grpc](https://github.com/grpc/grpc-go) | gRPC |
| `grpc_fib` | [github.com/lesismal/fib/grpc](https://github.com/lesismal/fib/tree/main/grpc) | gRPC, on fib's engine and HTTP/2 transport |
| `kitex` | [github.com/cloudwego/kitex](https://github.com/cloudwego/kitex) | Kitex Protobuf, pooled long connections |
| `kitex_mux` | [github.com/cloudwego/kitex](https://github.com/cloudwego/kitex) | Kitex Protobuf over TTHeader, mux transport |
| `rpcx` | [github.com/smallnest/rpcx](https://github.com/smallnest/rpcx) | rpcx |

## How it measures

The method is [kitex-benchmark](https://github.com/cloudwego/kitex-benchmark)'s:
each framework is benchmarked with its own client against its own server
(`frameworks/<name>/client` and `frameworks/<name>/server`), and every call is
an `Echo` of the same protobuf messages ([proto/echo.proto](proto/echo.proto)):
a `Request{Action, Msg}` the server sends back as a `Response`. The shared
client, [runner](runner/runner.go), does the rest the same way for all of them:

1. **Connect.** The client opens `-conns` connections, and one call is made
   over each, so a framework that dials lazily has dialed all of them before
   anything is timed.
2. **Warm up** for `-warmup` (5s), not measured.
3. **Measure** for `-d` (10s): `-c` goroutines call in a loop, one call at a
   time each, goroutine `i` over connection `i % conns`. `-c` defaults to the
   larger of `-conns` and 100: 10 calls in flight on each of 10 connections,
   and one in flight on each connection from 100 up.

Every call echoes a `-b` byte payload (1024) with a `-timeout` (5s), and a
call that errs, times out or comes back with a different payload counts as
failed. A run with a failed call or connection exits non-zero.

The client samples the server's CPU and memory (RSS) every second while it
measures, and writes a JSON report; `report` turns a run's reports into
Markdown in [go-websocket-benchmark](https://github.com/lesismal/go-websocket-benchmark)'s
format:

- **Summary**: what the run measured, one parameter a row.
- **TPS**: every framework's TPS at each connection count, when the run
  covered more than one.
- **Echo: N Conns**, one table per connection count, with:
  - **TPS [↓1]**: successful calls per second.
  - **CPU EER [↓2]**: TPS for each percent of a CPU the server spent on
    average, `TPS / CPU Avg`.
  - **MEM EER [↓3]**: TPS for each MB of memory the server held on average,
    `TPS / MEM Avg`.
  - **Min, Avg, Max, TP99, TP999**: the round trip of the successful calls,
    from a log-linear histogram accurate to about 1.6%.
  - **Total, Success, Failed**: the calls made.
  - **CPU Avg/Max**: the server's CPU, in percent of one CPU, over the whole
    run and in its busiest second.
  - **MEM Avg/Max**: the server's resident set.

  Rows are ranked by TPS, a tie broken by CPU EER and that by MEM EER, the
  columns marked [↓1], [↓2] and [↓3]; each of those cells carries its share of
  the best in its column. `BENCH_REPORT_SORT=framework` orders the rows by name
  instead.

The console shows each run as it ends, one value a line - the table's and
TP50 to TP95, the measured time, CPU Min, MEM Min, the client's own CPU and
memory, and **Server Conns**: the established connections the server held, as
the operating system counts them, which shows whether the client really
opened as many as it was asked to. The JSON report carries them too. Then
comes the report, each table padded to line up, after a rule and its name in
brackets. `output/report/report.md` has the
same tables under Markdown headings.

The server and the client run on separate CPUs (taskset, on Linux), and each
server runs alone, started just before its turn and stopped right after.

### Framework settings

Each framework runs its defaults, except where a fixed connection count or
protobuf needs otherwise, and except for arpc, which runs as kitex-benchmark
runs it:

- `arpc` and `arpc_fib` handle every request asynchronously, off the
  goroutine that read it (`SetAsyncResponse(true)`, and the Echo route
  registered with `async` true, which is both packages' default): arpc runs
  each one in a goroutine of its own, fib's arpc as a task on the engine's
  handler pool.
- `arpc`: pooled message buffers (`EnablePool`), as kitex-benchmark runs it;
  one `arpc.Client` per connection.
- `arpc_fib` and `grpc_fib`: fib's defaults; `grpc_fib` too answers every
  call on the engine's handler pool. The clients' connections share one fib
  engine.
- `grpc`: one `grpc.ClientConn` per connection.
- `kitex` and `kitex_mux` are kitex-benchmark's kitex and kitex-mux. A kitex
  client picks the connection of every call itself, so neither follows the
  runner's goroutine-to-connection mapping:
  - `kitex` runs kitex's default transport, on which a connection carries one
    call at a time: the client's long connection pool
    (`client.WithLongConnection`) holds one connection per call in flight,
    which is the concurrency rather than `-conns` - 100 connections at 10, as
    its run's Server Conns shows. The pool keeps that many idle, so none is
    closed and dialed again between two calls.
  - `kitex_mux` multiplexes calls over exactly `-conns` connections
    (`client.WithMuxConnection(conns)` with `server.WithMuxTransport()`),
    round robin; kitex no longer maintains this transport. Its connections are
    dialed lazily, so the client fills them all with calls one at a time
    before it is measured.
- `rpcx`: one `client.Client` per connection, the connection rpcx's
  `XClient` pools are made of; protobuf serialization.

## Running it

Go 1.27 or newer.

```sh
# Every framework at every connection count, on this machine
bash script/benchmark.sh

# Some of them, briefly, with client flags of your own
BENCH_CONNS=100,1000 BENCH_FRAMEWORKS=arpc,arpc_fib BENCH_DURATION=5s \
bash script/benchmark.sh -b=4096
```

[script/config.sh](script/config.sh) lists every `BENCH_*` setting:
`BENCH_FRAMEWORKS`, `BENCH_CONNS`, `BENCH_CONCURRENCY`, `BENCH_PAYLOAD`,
`BENCH_DURATION`, `BENCH_WARMUP`, `BENCH_SLEEP` (milliseconds the server holds
every call, kitex-benchmark's `-sleep`), `BENCH_TIMEOUT`, `BENCH_PROJECT` and
`BENCH_REPORT_SORT` for the report, and
`BENCH_SERVER_CPU_LIST`/`BENCH_CLIENT_CPU_LIST` for the CPUs each side is
pinned to (by default, half each). Flags after the script go to every client,
after its own, so they override them; `go run ./frameworks/arpc/client -h`
lists them.

The report is written to `output/report/report.md`, from the clients' JSON
reports beside it, and the servers' and clients' logs to `output/log`.
`bash script/report.sh` writes the report again from the JSON, and takes the
report's flags: `bash script/report.sh -sort=framework`.

10000 connections are 20000 open files, both ends being on one machine; the
script raises the soft limit as far as the hard one allows, and says so if
that is not enough.

### Docker

The Docker runner builds an image with everything in it, runs the benchmark
in a container with `--network none`, about 75% of the CPUs Docker exposes
(split evenly between the servers and the clients) and 80% of its memory, and
copies the report, the logs and the resources it ran with to
`output/docker/<timestamp>`:

```sh
# Every framework briefly, at 10 connections
bash script/docker_benchmark.sh --smoke

# The full benchmark
bash script/docker_benchmark.sh

# 8 CPUs: 4 for the servers, 4 for the clients
BENCH_CONNS=10000 bash script/docker_benchmark.sh -cput=8 -cpus=4 -cpuc=4
```

`bash script/docker_benchmark.sh --help` lists the rest. From mainland China,
`script/docker_benchmark_cn.sh` takes the same options and builds the image
from mirrors (DaoCloud for Docker Hub, Aliyun for apt, goproxy.cn for Go
modules).

### CI

[.github/workflows/docker-benchmark.yml](.github/workflows/docker-benchmark.yml)
runs the Docker benchmark on every push to main, one job per connection count,
one after another - CI's own set, 2, 10, 100, 500 and 1000 (its `conns`
input), rather than `BENCH_CONNS`'s 10 to 10000 - and writes each job's
report to its summary and its results to an artifact. It can also be started
from the Actions tab, as a smoke run or a full one, for some of the frameworks
or with other settings. It asks for 8 CPUs: set the repository variable
`DOCKER_BENCH_RUNNER` to the label of a runner that has them, or it runs on the
4 CPUs of `ubuntu-24.04` and says so in the summary.

## Code

| Path | What it is |
| --- | --- |
| [proto](proto) | `echo.proto`, and `gen.sh`, which generates the code below from it |
| [pb/echo](pb/echo) | its messages and grpc-go service |
| [pb/fibgrpc](pb/fibgrpc) | its service for fib's grpc |
| [kitex_gen](kitex_gen) | its messages and service for kitex |
| [codec/pbcodec](codec/pbcodec) | the protobuf codec arpc, fib's arpc and fib's grpc are given |
| [service](service) | what the servers share: the Echo work, `-addr`, the signal that stops them |
| [runner](runner) | the client every framework shares |
| [frameworks](frameworks) | each framework's server and client |
| [report](report) | the command that writes the report |
| [script](script) | the scripts above |

To add a framework, write its `frameworks/<name>/server` and
`frameworks/<name>/client` (a `runner.Client`), and give it a port in
`script/config.sh`'s `framework_port` (and `bench_reserved_ports`) and a place
in its `frameworks` list.
