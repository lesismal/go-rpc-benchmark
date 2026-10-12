package runner

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rules between the parts of the console output, go-websocket-benchmark's:
// ShortLine around one run, LongLine around the report's sections.
var (
	ShortLine = strings.Repeat("-", 62) + "\n"
	LongLine  = strings.Repeat("-", 100) + "\n"
)

// FormatDuration is lesismal/perf's I2TimeString: two decimals of the largest
// unit under which the value is at least 1; "-" for none.
func FormatDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d)
	case d < time.Millisecond:
		return fmt.Sprintf("%.2fus", float64(d)/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/1e6)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// FormatSeconds is FormatDuration of s seconds.
func FormatSeconds(s float64) string {
	return FormatDuration(time.Duration(s * float64(time.Second)))
}

// FormatCPU is a CPU in percent of one CPU.
func FormatCPU(v float64) string {
	return fmt.Sprintf("%.2f%%", v)
}

// FormatMem is lesismal/perf's I2MemString: two decimals of K, M or G; "-"
// for none.
func FormatMem(v uint64) string {
	switch {
	case v == 0:
		return "-"
	case v >= 1<<30:
		return fmt.Sprintf("%.2fG", float64(v)/(1<<30))
	case v >= 1<<20:
		return fmt.Sprintf("%.2fM", float64(v)/(1<<20))
	default:
		return fmt.Sprintf("%.2fK", float64(v)/(1<<10))
	}
}

// String is the run as go-websocket-benchmark prints one to the console when
// it ends: a name and a value a line, the names padded to one width.
func (r *Result) String() string {
	failed := strconv.FormatUint(r.Failed, 10)
	if r.ConnFailed > 0 {
		failed += fmt.Sprintf(" (%d connections failed)", r.ConnFailed)
	}
	serverConns := "-"
	if r.ServerConns >= 0 {
		serverConns = strconv.Itoa(r.ServerConns)
	}
	sleep := "0ms"
	if r.SleepMs > 0 {
		sleep = strconv.Itoa(r.SleepMs) + "ms"
	}
	lines := [][2]string{
		{"BenchType", "Echo"},
		{"Framework", r.Framework},
		{"TPS", fmt.Sprintf("%.0f", r.TPS)},
		{"CPU EER", fmt.Sprintf("%.2f", r.CPUEER)},
		{"MEM EER", fmt.Sprintf("%.2f", r.MEMEER)},
		{"Min", FormatDuration(r.Min)},
		{"Avg", FormatDuration(r.Avg)},
		{"Max", FormatDuration(r.Max)},
		{"TP50", FormatDuration(r.TP50)},
		{"TP75", FormatDuration(r.TP75)},
		{"TP90", FormatDuration(r.TP90)},
		{"TP95", FormatDuration(r.TP95)},
		{"TP99", FormatDuration(r.TP99)},
		{"TP999", FormatDuration(r.TP999)},
		{"Used", FormatSeconds(r.Used)},
		{"Total", strconv.FormatUint(r.Total, 10)},
		{"Success", strconv.FormatUint(r.Success, 10)},
		{"Failed", failed},
		{"Conns", strconv.Itoa(r.Conns)},
		{"Server Conns", serverConns},
		{"Connect Time", FormatSeconds(r.ConnectTime)},
		{"Concurrency", strconv.Itoa(r.Concurrency)},
		{"Payload", strconv.Itoa(r.Payload)},
		{"Server Sleep", sleep},
		{"CPU Min", FormatCPU(r.Server.CPUMin)},
		{"CPU Avg", FormatCPU(r.Server.CPUAvg)},
		{"CPU Max", FormatCPU(r.Server.CPUMax)},
		{"MEM Min", FormatMem(r.Server.MEMMin)},
		{"MEM Avg", FormatMem(r.Server.MEMAvg)},
		{"MEM Max", FormatMem(r.Server.MEMMax)},
		{"Client CPU Avg", FormatCPU(r.Client.CPUAvg)},
		{"Client MEM Max", FormatMem(r.Client.MEMMax)},
	}
	width := 0
	for _, l := range lines {
		width = max(width, len(l[0]))
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l[0] + strings.Repeat(" ", width-len(l[0])) + ": " + l[1] + "\n")
	}
	return b.String()
}
