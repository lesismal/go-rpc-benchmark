package runner

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Usage is what a process used while the calls were measured, sampled every
// interval: its CPU in percent of one CPU (250 is two and a half CPUs kept
// busy), and its resident set in bytes.
//
// CPUMin and CPUMax are the least and most of any one interval. CPUAvg is the
// CPU time of the whole run over its wall time, which is the mean of the
// intervals without the error of their edges. MEMAvg is the mean of the
// samples.
type Usage struct {
	CPUMin float64
	CPUAvg float64
	CPUMax float64
	MEMMin uint64
	MEMAvg uint64
	MEMMax uint64
}

// sampler measures a process from start to finish.
type sampler struct {
	proc  *process.Process
	begin time.Time
	cpu0  float64
	stop  chan struct{}
	done  sync.WaitGroup

	// The last sample's time and CPU time, and the samples so far.
	at     time.Time
	cpu    float64
	cpus   []float64
	rss    []uint64
	failed bool
}

func cpuTime(p *process.Process) (float64, bool) {
	t, err := p.Times()
	if err != nil {
		return 0, false
	}
	return t.User + t.System, true
}

// startSampler starts measuring pid, or returns nil when it cannot be read;
// a nil sampler's finish reports nothing.
func startSampler(pid int, interval time.Duration) *sampler {
	if pid <= 0 {
		return nil
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil
	}
	cpu0, ok := cpuTime(p)
	if !ok {
		return nil
	}
	now := time.Now()
	s := &sampler{proc: p, begin: now, cpu0: cpu0, at: now, cpu: cpu0, stop: make(chan struct{})}
	s.sampleRSS()
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.sample(0)
			}
		}
	}()
	return s
}

func (s *sampler) sampleRSS() {
	if m, err := s.proc.MemoryInfo(); err == nil {
		s.rss = append(s.rss, m.RSS)
	}
}

// sample records the CPU of the interval since the last sample, if it is at
// least minInterval long, and the resident set.
func (s *sampler) sample(minInterval time.Duration) {
	now := time.Now()
	cpu, ok := cpuTime(s.proc)
	if !ok {
		s.failed = true
		return
	}
	if wall := now.Sub(s.at); wall > 0 && wall >= minInterval {
		s.cpus = append(s.cpus, (cpu-s.cpu)/wall.Seconds()*100)
		s.at, s.cpu = now, cpu
	}
	s.sampleRSS()
}

func (s *sampler) finish() Usage {
	if s == nil {
		return Usage{}
	}
	close(s.stop)
	s.done.Wait()
	// The last stretch, unless it is too short to say anything on its own.
	s.sample(200 * time.Millisecond)

	var u Usage
	if cpu1, ok := cpuTime(s.proc); ok && !s.failed {
		if wall := time.Since(s.begin).Seconds(); wall > 0 {
			u.CPUAvg = (cpu1 - s.cpu0) / wall * 100
		}
	}
	u.CPUMin, u.CPUMax = u.CPUAvg, u.CPUAvg
	for i, c := range s.cpus {
		if i == 0 || c < u.CPUMin {
			u.CPUMin = c
		}
		if i == 0 || c > u.CPUMax {
			u.CPUMax = c
		}
	}
	var sum uint64
	for i, m := range s.rss {
		sum += m
		if i == 0 || m < u.MEMMin {
			u.MEMMin = m
		}
		if m > u.MEMMax {
			u.MEMMax = m
		}
	}
	if len(s.rss) > 0 {
		u.MEMAvg = sum / uint64(len(s.rss))
	}
	return u
}

// established counts pid's established TCP connections whose local port is
// port: the connections a server holds from the client, which says whether
// the client really opened as many as it was asked to.
func established(pid int, port uint32) int {
	if pid <= 0 {
		return -1
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return -1
	}
	conns, err := p.Connections()
	if err != nil {
		return -1
	}
	n := 0
	for _, c := range conns {
		if c.Status == "ESTABLISHED" && c.Laddr.Port == port {
			n++
		}
	}
	return n
}
