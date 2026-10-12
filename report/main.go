// Command report turns the JSON reports of a run's clients into Markdown
// tables, in go-websocket-benchmark's format, which it prints to the console,
// each section after a rule and its name in brackets, and writes to a file,
// each section under a heading:
//
//   - Summary: what the run measured, one parameter a row.
//   - TPS: every framework's TPS at each connection count, when the run
//     covered more than one.
//   - One Echo table per connection count (and payload and server sleep, when
//     a run varies them), ranked by TPS [↓1], then CPU EER [↓2], then
//     MEM EER [↓3], each of those cells with its share of the best.
//
// Usage:
//
//	go run ./report -dir output/report -out output/report/report.md
package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go-rpc-benchmark/runner"
)

// The orders a table's rows can be written in, as -sort takes them.
const (
	// sortResult puts the best first: the highest TPS, a tie broken by the
	// higher CPU EER, and that by the higher MEM EER. Rows that tie on all
	// three keep the framework order.
	sortResult = "result"
	// sortFramework is framework name order, which puts a framework on the
	// same row in every table and every run, whatever it scored.
	sortFramework = "framework"
)

func main() {
	dir := flag.String("dir", "output/report", "directory of the clients' JSON reports")
	out := flag.String("out", "", "Markdown file to write (empty: stdout only)")
	project := flag.String("project", "GO-RPC-BENCHMARK", "the Summary's Project row; empty leaves it out")
	order := flag.String("sort", sortResult, "row order: result (best first) or framework (by name)")
	flag.Parse()
	if *order != sortResult && *order != sortFramework {
		fatal(fmt.Errorf("-sort must be %s or %s, got %q", sortResult, sortFramework, *order))
	}

	results, err := load(*dir)
	if err != nil {
		fatal(err)
	}
	sections := render(*project, *order, results)
	var console, file strings.Builder
	for _, s := range sections {
		console.WriteString(runner.LongLine + "[" + s.name + "]\n\n" + s.body + "\n")
		file.WriteString("## " + s.name + "\n\n" + s.body + "\n")
	}
	console.WriteString(runner.LongLine)
	fmt.Print(console.String())
	if *out != "" {
		if err := os.WriteFile(*out, []byte(file.String()), 0o644); err != nil {
			fatal(err)
		}
	}
}

// section is one table of the report, and what follows it.
type section struct {
	name, body string
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func load(dir string) ([]runner.Result, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var results []runner.Result
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var r runner.Result
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		results = append(results, r)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no reports in %s", dir)
	}
	// Framework order, which sortResult keeps between rows that tie.
	slices.SortStableFunc(results, func(a, b runner.Result) int { return cmp.Compare(a.Framework, b.Framework) })
	return results, nil
}

// group is the runs that were measured under the same load.
type group struct {
	Conns, Payload, SleepMs int
}

func render(project, order string, results []runner.Result) []section {
	byGroup := map[group][]runner.Result{}
	var groups []group
	for _, r := range results {
		g := group{r.Conns, r.Payload, r.SleepMs}
		if _, ok := byGroup[g]; !ok {
			groups = append(groups, g)
		}
		byGroup[g] = append(byGroup[g], r)
	}
	slices.SortFunc(groups, func(a, b group) int {
		return cmp.Or(cmp.Compare(a.Conns, b.Conns), cmp.Compare(a.Payload, b.Payload), cmp.Compare(a.SleepMs, b.SleepMs))
	})
	payloads, sleeps := distinct(results, func(r runner.Result) int { return r.Payload }),
		distinct(results, func(r runner.Result) int { return r.SleepMs })

	sections := []section{{"Summary", summary(project, results)}}
	if len(groups) > 1 {
		sections = append(sections, section{"TPS", tpsTable(groups, byGroup, payloads > 1, sleeps > 1)})
	}
	for _, g := range groups {
		rs := sortResults(byGroup[g], order)
		sections = append(sections, section{"Echo: " + g.title(payloads > 1, sleeps > 1), echoTable(rs) + errorList(rs)})
	}
	return sections
}

func (g group) title(payload, sleep bool) string {
	s := fmt.Sprintf("%d Conns", g.Conns)
	if payload {
		s += fmt.Sprintf(", %dB Payload", g.Payload)
	}
	if sleep {
		s += fmt.Sprintf(", %dms Sleep", g.SleepMs)
	}
	return s
}

func distinct(results []runner.Result, key func(runner.Result) int) int {
	seen := map[int]bool{}
	for _, r := range results {
		seen[key(r)] = true
	}
	return len(seen)
}

func sortResults(rs []runner.Result, order string) []runner.Result {
	rs = slices.Clone(rs)
	if order == sortResult {
		slices.SortStableFunc(rs, func(a, b runner.Result) int {
			return cmp.Or(cmp.Compare(b.TPS, a.TPS), cmp.Compare(b.CPUEER, a.CPUEER), cmp.Compare(b.MEMEER, a.MEMEER))
		})
	}
	return rs
}

// summaryParameters is the Summary's rows: the name, how a result says it,
// and the Description column.
var summaryParameters = []struct {
	name        string
	value       func(r runner.Result) string
	description string
}{
	{"Frameworks", func(r runner.Result) string { return r.Framework }, "frameworks in this report (BENCH_FRAMEWORKS)"},
	{"Conns", func(r runner.Result) string { return strconv.Itoa(r.Conns) }, "connections each benchmark runs over (BENCH_CONNS, -conns)"},
	{"Concurrency", func(r runner.Result) string { return strconv.Itoa(r.Concurrency) }, "goroutines calling at once, one call in flight each (BENCH_CONCURRENCY, -c)"},
	{"Payload", func(r runner.Result) string { return strconv.Itoa(r.Payload) }, "bytes every call echoes (BENCH_PAYLOAD, -b)"},
	{"Server Sleep", func(r runner.Result) string { return fmt.Sprintf("%dms", r.SleepMs) }, "how long the server holds every call (BENCH_SLEEP, -sleep)"},
	{"Warmup", func(r runner.Result) string { return seconds(r.Warmup) }, "calls before the measured ones, not counted (BENCH_WARMUP, -warmup)"},
	{"Duration", func(r runner.Result) string { return seconds(math.Round(r.Used)) }, "measured time of every benchmark (BENCH_DURATION, -d)"},
	{"Timeout", func(r runner.Result) string { return seconds(r.Timeout) }, "timeout of every call and dial (BENCH_TIMEOUT, -timeout)"},
	{"Server CPUs", func(r runner.Result) string { return orNone(r.ServerCPUs) }, "CPUs the servers are pinned to (BENCH_SERVER_CPU_LIST)"},
	{"Client CPUs", func(r runner.Result) string { return orNone(r.ClientCPUs) }, "CPUs the clients are pinned to (BENCH_CLIENT_CPU_LIST)"},
	{"Client GOMAXPROCS", func(r runner.Result) string { return strconv.Itoa(r.GOMAXPROCS) }, "Ps the clients run"},
}

func summary(project string, results []runner.Result) string {
	var rows [][]string
	if project != "" {
		rows = append(rows, []string{"Project", project, "what this run benchmarks (BENCH_PROJECT, -project)"})
	}
	for _, p := range summaryParameters {
		var values []string
		for _, r := range results {
			if v := p.value(r); !slices.Contains(values, v) {
				values = append(values, v)
			}
		}
		if p.name == "Conns" || p.name == "Concurrency" || p.name == "Payload" {
			slices.SortFunc(values, func(a, b string) int {
				x, _ := strconv.Atoi(a)
				y, _ := strconv.Atoi(b)
				return cmp.Compare(x, y)
			})
		}
		rows = append(rows, []string{p.name, strings.Join(values, ", "), p.description})
	}
	return markdownTableAligned([]string{"Parameter", "Value", "Description"}, rows, true)
}

func orNone(s string) string {
	if s == "" {
		return "not pinned"
	}
	return s
}

func seconds(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64) + "s"
}

// tpsTable is every framework's TPS at each load, so that how a framework
// holds up as connections grow reads along one row.
func tpsTable(groups []group, byGroup map[group][]runner.Result, payload, sleep bool) string {
	var names []string
	tps := map[string]map[group]float64{}
	for _, g := range groups {
		for _, r := range byGroup[g] {
			if tps[r.Framework] == nil {
				tps[r.Framework] = map[group]float64{}
				names = append(names, r.Framework)
			}
			tps[r.Framework][g] = r.TPS
		}
	}
	slices.Sort(names)
	title := []string{"Framework"}
	for _, g := range groups {
		title = append(title, g.title(payload, sleep))
	}
	rows := make([][]string, len(names))
	for i, n := range names {
		rows[i] = []string{n}
		for _, g := range groups {
			if v, ok := tps[n][g]; ok {
				rows[i] = append(rows[i], fmt.Sprintf("%.0f", v))
			} else {
				rows[i] = append(rows[i], "-")
			}
		}
	}
	for j := range groups {
		values := make([]float64, len(names))
		for i, n := range names {
			values[i] = tps[n][groups[j]]
		}
		withPercent(rows, j+1, values)
	}
	return markdownTable(title, rows)
}

// echoTable is one load's runs, in the order given. Its columns are
// go-websocket-benchmark's BenchEcho's: the JSON reports and each run's
// console output carry the rest (TP75, TP90, TP95, CPU Min, MEM Min and the
// client's own CPU and memory).
func echoTable(rs []runner.Result) string {
	title := []string{
		"Framework", "TPS" + rankMarker(1), "CPU EER" + rankMarker(2), "MEM EER" + rankMarker(3),
		"Min", "Avg", "Max", "TP50", "TP99", "TP999",
		"Used", "Total", "Success", "Failed", "Conns", "Server Conns",
		"CPU Avg", "CPU Max", "MEM Avg", "MEM Max",
	}
	rows := make([][]string, len(rs))
	tps := make([]float64, len(rs))
	cpuEER := make([]float64, len(rs))
	memEER := make([]float64, len(rs))
	for i, r := range rs {
		tps[i], cpuEER[i], memEER[i] = r.TPS, r.CPUEER, r.MEMEER
		failed := strconv.FormatUint(r.Failed, 10)
		if r.ConnFailed > 0 {
			failed = fmt.Sprintf("%d conns", r.ConnFailed)
		}
		serverConns := "-"
		if r.ServerConns >= 0 {
			serverConns = strconv.Itoa(r.ServerConns)
		}
		rows[i] = []string{
			r.Framework,
			fmt.Sprintf("%.0f", r.TPS), fmt.Sprintf("%.2f", r.CPUEER), fmt.Sprintf("%.2f", r.MEMEER),
			runner.FormatDuration(r.Min), runner.FormatDuration(r.Avg), runner.FormatDuration(r.Max),
			runner.FormatDuration(r.TP50), runner.FormatDuration(r.TP99), runner.FormatDuration(r.TP999),
			runner.FormatSeconds(r.Used),
			strconv.FormatUint(r.Total, 10), strconv.FormatUint(r.Success, 10), failed,
			strconv.Itoa(r.Conns), serverConns,
			runner.FormatCPU(r.Server.CPUAvg), runner.FormatCPU(r.Server.CPUMax),
			runner.FormatMem(r.Server.MEMAvg), runner.FormatMem(r.Server.MEMMax),
		}
	}
	withPercent(rows, 1, tps)
	withPercent(rows, 2, cpuEER)
	withPercent(rows, 3, memEER)
	return markdownTable(title, rows)
}

// errorList is a few of every framework's errors: they are mostly the same.
func errorList(rs []runner.Result) string {
	var b strings.Builder
	for _, r := range rs {
		for _, e := range r.Errors[:min(len(r.Errors), 3)] {
			fmt.Fprintf(&b, "- %s: `%s`\n", r.Framework, strings.ReplaceAll(e, "`", "'"))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n" + b.String()
}

// rankMarker is what a rank column's title carries: [↓1] on the key the rows
// are ranked by, highest first, [↓2] on the one that breaks a tie on it, and
// [↓3] on the one that breaks a tie on both.
func rankMarker(rank int) string {
	return " [↓" + strconv.Itoa(rank) + "]"
}

// percent is how a value reads against the best in its column: floored, so
// that only the best shows 100%, and 0% when nothing scored.
func percent(value, best float64) string {
	if best <= 0 || value <= 0 {
		return "0%"
	}
	if value >= best {
		return "100%"
	}
	p := int64(math.Floor(value*100/best + 1e-9))
	return fmt.Sprintf("%d%%", min(p, 99))
}

// withPercent appends each row's share of the best to its cell in column
// col, the value and the share each right-aligned, so that every cell of the
// column has one width:
//
//	1000000 100%
//	 123456  12%
func withPercent(rows [][]string, col int, values []float64) {
	best := 0.0
	for _, v := range values {
		best = math.Max(best, v)
	}
	percents := make([]string, len(rows))
	valueLen, percentLen := 0, 0
	for i, row := range rows {
		percents[i] = percent(values[i], best)
		valueLen = max(valueLen, len(row[col]))
		percentLen = max(percentLen, len(percents[i]))
	}
	for i, row := range rows {
		row[col] = strings.Repeat(" ", valueLen-len(row[col])) + row[col] + " " +
			strings.Repeat(" ", percentLen-len(percents[i])) + percents[i]
	}
}
