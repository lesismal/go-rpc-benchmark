// Package runner is the benchmark client every framework shares, after
// kitex-benchmark's runner: a framework supplies the calls, and the runner
// opens the connections, loads them from a fixed number of goroutines for a
// fixed time, and reports the throughput, the latency and what the server
// used doing it.
//
// A run has three phases:
//
//  1. Connect. The framework's client opens -conns connections, and one
//     call is made over each of them, so a framework that dials lazily has
//     dialed every one before anything is timed.
//  2. Warm up. -c goroutines call for -warmup, which is not measured.
//  3. Measure. The same goroutines call for -d. Goroutine i calls over
//     connection i % -conns, one call at a time: with -c equal to -conns
//     every connection carries one call at a time, and with -c a multiple of
//     it, that many at once over the same connection.
//
// Every call echoes a -b byte payload and is checked: an error, a timeout
// (-timeout) or a reply that is not the payload is a failed call.
package runner

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-rpc-benchmark/service"
)

// Options is what a framework's client is built from.
type Options struct {
	// Addr is the server's host:port.
	Addr string
	// Conns is how many connections to open.
	Conns int
	// Concurrency is how many goroutines call at once, which a framework
	// that holds a connection per call in flight sizes its pool by.
	Concurrency int
	// Timeout bounds every call, and the dial of every connection.
	Timeout time.Duration
	// DialParallel is how many connections may be dialing at once.
	DialParallel int
}

// Client is one framework's client: Options.Conns connections to the server.
type Client interface {
	// Echo makes an Echo call over connection conn, 0 <= conn < Options.Conns,
	// and returns the reply's Msg. It is called from many goroutines at once.
	Echo(conn int, action, msg string) (string, error)
	// Close closes every connection.
	Close() error
}

// NewClient builds a framework's Client and opens its connections, or as
// many of them as the framework opens before its first call.
type NewClient func(opts Options) (Client, error)

// Result is the report of one run, which script/report.sh reads back.
type Result struct {
	Framework string

	// The load: Conns connections, Concurrency goroutines calling at once,
	// Payload bytes a call, SleepMs the server holds every call for, Warmup
	// seconds before the Used seconds measured, and Timeout seconds a call.
	Conns       int
	Concurrency int
	Payload     int
	SleepMs     int
	Warmup      float64
	Used        float64
	Timeout     float64
	// Where it ran: the CPUs the server and the client were pinned to
	// (-servercpus and -clientcpus, as the scripts pinned them), and the
	// client's GOMAXPROCS.
	ServerCPUs string
	ClientCPUs string
	GOMAXPROCS int

	ConnectTime float64 // seconds to open the connections and call over each
	ConnFailed  int     // connections that could not be opened or called over
	// ServerConns is the established connections the server held during the
	// run, as the operating system counts them; -1 when it could not be read.
	ServerConns int

	Total   uint64
	Success uint64
	Failed  uint64
	// TPS is the successful calls a second.
	TPS float64
	// CPUEER is TPS for each percent of a CPU the server spent on average,
	// and MEMEER TPS for each MB of memory it held on average; see EER.
	CPUEER float64
	MEMEER float64
	Latency

	Server Usage
	Client Usage

	Errors []string `json:",omitempty"`
}

// EER is go-websocket-benchmark's energy efficiency ratio: the throughput a
// server got for each unit of what it spent, or 0 when it spent nothing that
// was measured. CPU EER is TPS over the server's CPUAvg in percent of a CPU,
// MEM EER TPS over its MEMAvg in MB (1024*1024 bytes).
func EER(throughput, spent float64) float64 {
	if spent <= 0 || math.IsNaN(spent) || math.IsInf(throughput, 0) || math.IsNaN(throughput) {
		return 0
	}
	eer := throughput / spent
	if math.IsInf(eer, 0) || math.IsNaN(eer) {
		return 0
	}
	return eer
}

// MEMEER is EER over memory in MB.
func MEMEER(throughput float64, memAvg uint64) float64 {
	return EER(throughput, float64(memAvg)/(1024*1024))
}

// errorLog keeps the first few distinct errors of a run for the report.
type errorLog struct {
	mu   sync.Mutex
	seen map[string]bool
	list []string
}

func (l *errorLog) add(err error) {
	msg := err.Error()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.list) >= 10 || l.seen[msg] {
		return
	}
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	l.seen[msg] = true
	l.list = append(l.list, msg)
}

// reportFile is where finish writes the report: -out.
var reportFile string

var errMismatch = errors.New("reply does not match the request")

// Main runs the benchmark for the framework called name, and exits.
func Main(name string, newClient NewClient) {
	var (
		addr         = flag.String("addr", "127.0.0.1:20001", "server address")
		conns        = flag.Int("conns", 100, "connections to open")
		concurrency  = flag.Int("c", 0, "goroutines calling at once (0: the larger of -conns and 100)")
		payload      = flag.Int("b", 1024, "payload size in bytes")
		duration     = flag.Duration("d", 10*time.Second, "measured duration")
		warmup       = flag.Duration("warmup", 5*time.Second, "warm-up duration, not measured")
		timeout      = flag.Duration("timeout", 5*time.Second, "timeout of every call and dial")
		sleepMs      = flag.Int("sleep", 0, "milliseconds the server holds every call (0: echo at once)")
		dialParallel = flag.Int("dialc", 256, "connections dialing at once")
		serverPid    = flag.Int("spid", 0, "server process to measure CPU and memory of (0: none)")
		serverCPUs   = flag.String("servercpus", "", "the CPUs the server is pinned to, for the report only")
		clientCPUs   = flag.String("clientcpus", "", "the CPUs this client is pinned to, for the report only")
	)
	flag.StringVar(&reportFile, "out", "", "file to write the JSON report to (empty: stdout only)")
	flag.Parse()
	c := *concurrency
	if c <= 0 {
		c = max(*conns, 100)
	}
	if *conns <= 0 {
		fatalf("-conns must be positive")
	}

	action, msg := service.ActionEcho, strings.Repeat("x", *payload)
	if *sleepMs > 0 {
		action = service.ActionSleep
		prefix := strconv.Itoa(*sleepMs) + ","
		msg = prefix + msg[min(len(prefix), len(msg)):]
	}

	res := &Result{
		Framework:   name,
		Conns:       *conns,
		Concurrency: c,
		Payload:     *payload,
		SleepMs:     *sleepMs,
		Warmup:      warmup.Seconds(),
		Timeout:     timeout.Seconds(),
		ServerCPUs:  *serverCPUs,
		ClientCPUs:  *clientCPUs,
		GOMAXPROCS:  runtime.GOMAXPROCS(0),
		ServerConns: -1,
	}
	var errs errorLog
	logf("%s: %d connections to %s, %d goroutines, %d byte payload, %v warm-up, %v measured",
		name, *conns, *addr, c, *payload, *warmup, *duration)

	// === connect ===
	begin := time.Now()
	client, err := newClient(Options{Addr: *addr, Conns: *conns, Concurrency: c, Timeout: *timeout, DialParallel: *dialParallel})
	if err != nil {
		errs.add(err)
		res.ConnFailed = *conns
		res.Errors = errs.list
		finish(res, fmt.Errorf("connect: %w", err))
	}
	var connFailed atomic.Int64
	Parallel(*conns, *dialParallel, func(i int) error {
		reply, err := client.Echo(i, service.ActionEcho, "ping")
		if err == nil && reply != "ping" {
			err = errMismatch
		}
		if err != nil {
			connFailed.Add(1)
			errs.add(fmt.Errorf("connection %d: %w", i, err))
		}
		return nil
	})
	res.ConnectTime = time.Since(begin).Seconds()
	res.ConnFailed = int(connFailed.Load())
	logf("%s: connected in %.2fs, %d failed", name, res.ConnectTime, res.ConnFailed)
	if res.ConnFailed > 0 {
		client.Close()
		res.Errors = errs.list
		finish(res, fmt.Errorf("%d of %d connections failed", res.ConnFailed, *conns))
	}

	// === warm up ===
	if *warmup > 0 {
		load(client, c, *conns, *warmup, action, msg, newHistogram(1), nil)
	}

	// === measure ===
	hist := newHistogram(max(64, 4*runtime.GOMAXPROCS(0)))
	srv := startSampler(*serverPid, time.Second)
	self := startSampler(os.Getpid(), time.Second)
	elapsed := load(client, c, *conns, *duration, action, msg, hist, &errs)
	res.Server = srv.finish()
	res.Client = self.finish()
	if _, port, err := net.SplitHostPort(*addr); err == nil {
		if p, err := strconv.ParseUint(port, 10, 16); err == nil {
			res.ServerConns = established(*serverPid, uint32(p))
		}
	}
	client.Close()

	res.Used = elapsed.Seconds()
	res.Total, res.Failed, res.Latency = hist.summary()
	res.Success = res.Total - res.Failed
	if res.Used > 0 {
		res.TPS = float64(res.Success) / res.Used
	}
	res.CPUEER = EER(res.TPS, res.Server.CPUAvg)
	res.MEMEER = MEMEER(res.TPS, res.Server.MEMAvg)
	res.Errors = errs.list
	if res.Failed > 0 {
		finish(res, fmt.Errorf("%d of %d calls failed", res.Failed, res.Total))
	}
	finish(res, nil)
}

// load runs c goroutines calling for d and returns how long they took, from
// the first call to the end of the last one. A nil errs drops the errors.
func load(client Client, c, conns int, d time.Duration, action, msg string, hist *histogram, errs *errorLog) time.Duration {
	var stop atomic.Bool
	var wg sync.WaitGroup
	timer := time.AfterFunc(d, func() { stop.Store(true) })
	defer timer.Stop()
	begin := time.Now()
	for w := 0; w < c; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			conn, s := w%conns, hist.shard(w)
			for !stop.Load() {
				t := time.Now()
				reply, err := client.Echo(conn, action, msg)
				cost := time.Since(t)
				if err == nil && reply != msg {
					err = errMismatch
				}
				s.record(cost, err != nil)
				if err != nil && errs != nil {
					errs.add(err)
				}
			}
		}(w)
	}
	wg.Wait()
	return time.Since(begin)
}

// Parallel calls f(i) for every i in [0, n), at most limit at once, and
// returns the first error any of them returned.
func Parallel(n, limit int, f func(i int) error) error {
	if limit <= 0 {
		limit = 1
	}
	var (
		wg    sync.WaitGroup
		next  atomic.Int64
		first atomic.Pointer[error]
	)
	for range min(n, limit) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				if err := f(i); err != nil {
					first.CompareAndSwap(nil, &err)
				}
			}
		}()
	}
	wg.Wait()
	if p := first.Load(); p != nil {
		return *p
	}
	return nil
}

// finish writes the report and exits: 1 when err says the run failed, whose
// report is still written so that the report step shows it.
func finish(res *Result, err error) {
	// The run as go-websocket-benchmark prints one, to the console: stdout
	// carries the JSON, which the scripts keep out of the console.
	fmt.Fprint(os.Stderr, ShortLine+res.String())
	data, jerr := json.Marshal(res)
	if jerr != nil {
		fatalf("encode report: %v", jerr)
	}
	fmt.Println(string(data))
	if reportFile != "" {
		if werr := os.MkdirAll(filepath.Dir(reportFile), 0o755); werr != nil {
			fatalf("write report: %v", werr)
		}
		if werr := os.WriteFile(reportFile, append(data, '\n'), 0o644); werr != nil {
			fatalf("write report: %v", werr)
		}
	}
	if err != nil {
		fatalf("%s: %v", res.Framework, err)
	}
	os.Exit(0)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}

func fatalf(format string, args ...any) {
	logf(format, args...)
	os.Exit(1)
}
