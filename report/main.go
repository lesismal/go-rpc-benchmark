// Command report turns the JSON reports of a run's clients into Markdown
// tables: one per connection count (and payload, concurrency and sleep, when
// a run varies them), best TPS first, and, when the run covered more than one
// connection count, a summary of every framework's TPS at each.
//
//	go run ./report -dir output/report -out output/report/report.md
package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go-rpc-benchmark/runner"
)

func main() {
	dir := flag.String("dir", "output/report", "directory of the clients' JSON reports")
	out := flag.String("out", "", "Markdown file to write (empty: stdout only)")
	title := flag.String("title", "GO-RPC-BENCHMARK", "heading of the report; empty leaves it out")
	flag.Parse()

	results, err := load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	md := render(*title, results)
	fmt.Print(md)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
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
	return results, nil
}

// group is the runs that were measured under the same load.
type group struct {
	Conns, Concurrency, Payload, SleepMs int
}

func (g group) title() string {
	s := fmt.Sprintf("%d connections, %d concurrency, %d byte payload", g.Conns, g.Concurrency, g.Payload)
	if g.SleepMs > 0 {
		s += fmt.Sprintf(", %dms server sleep", g.SleepMs)
	}
	return s
}

func render(title string, results []runner.Result) string {
	byGroup := map[group][]runner.Result{}
	for _, r := range results {
		g := group{r.Conns, r.Concurrency, r.Payload, r.SleepMs}
		byGroup[g] = append(byGroup[g], r)
	}
	groups := make([]group, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	slices.SortFunc(groups, func(a, b group) int {
		return cmp.Or(cmp.Compare(a.Conns, b.Conns), cmp.Compare(a.Payload, b.Payload),
			cmp.Compare(a.Concurrency, b.Concurrency), cmp.Compare(a.SleepMs, b.SleepMs))
	})

	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "# %s\n\n", title)
	}
	if len(groups) > 1 {
		summary(&b, groups, byGroup)
	}
	for _, g := range groups {
		table(&b, g, byGroup[g])
	}
	return b.String()
}

// table writes one load's runs, best TPS first.
func table(b *strings.Builder, g group, rs []runner.Result) {
	slices.SortStableFunc(rs, func(x, y runner.Result) int {
		return cmp.Or(cmp.Compare(y.TPS, x.TPS), cmp.Compare(x.Framework, y.Framework))
	})
	best := rs[0].TPS
	fmt.Fprintf(b, "## %s\n\n", g.title())
	b.WriteString("| Framework | TPS | AVG | P50 | P90 | P99 | P999 | MAX | Server CPU | TPS/CPU | Server RSS | Client CPU | Server Conns | Failed |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, r := range rs {
		share := "-"
		if best > 0 {
			share = fmt.Sprintf("%.0f%%", r.TPS/best*100)
		}
		perCPU := "-"
		if r.Server.CPU > 0 {
			perCPU = fmt.Sprintf("%.0f", r.TPS/(r.Server.CPU/100))
		}
		failed := fmt.Sprintf("%d", r.Failed)
		if r.ConnFailed > 0 {
			failed = fmt.Sprintf("%d conns", r.ConnFailed)
		}
		conns := "-"
		if r.ServerConns >= 0 {
			conns = fmt.Sprintf("%d", r.ServerConns)
		}
		fmt.Fprintf(b, "| %s | %.0f (%s) | %s | %s | %s | %s | %s | %s | %.0f%% | %s | %s | %.0f%% | %s | %s |\n",
			r.Framework, r.TPS, share,
			dur(r.Avg), dur(r.P50), dur(r.P90), dur(r.P99), dur(r.P999), dur(r.Max),
			r.Server.CPU, perCPU, mib(r.Server.RSSMax), r.Client.CPU, conns, failed)
	}
	b.WriteString("\n")
	// A few of each framework's errors: they are mostly the same one.
	for _, r := range rs {
		for _, e := range r.Errors[:min(len(r.Errors), 3)] {
			fmt.Fprintf(b, "- %s: `%s`\n", r.Framework, strings.ReplaceAll(e, "`", "'"))
		}
	}
	b.WriteString("\n")
}

// summary writes every framework's TPS at each load, so that how a framework
// holds up as connections grow reads along one row.
func summary(b *strings.Builder, groups []group, byGroup map[group][]runner.Result) {
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
	b.WriteString("## TPS\n\n| Framework |")
	for _, g := range groups {
		label := fmt.Sprintf(" %d conns", g.Conns)
		if g.SleepMs > 0 {
			label += fmt.Sprintf(" (%dms)", g.SleepMs)
		}
		b.WriteString(label + " |")
	}
	b.WriteString("\n| --- |" + strings.Repeat(" ---: |", len(groups)) + "\n")
	for _, n := range names {
		b.WriteString("| " + n + " |")
		for _, g := range groups {
			if v, ok := tps[n][g]; ok {
				fmt.Fprintf(b, " %.0f |", v)
			} else {
				b.WriteString(" - |")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func dur(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func mib(n uint64) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
}
