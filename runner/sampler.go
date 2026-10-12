package runner

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Usage is what a process used while the calls were measured.
type Usage struct {
	// CPU is the CPU time it took over the wall time, in percent of one
	// CPU: 250 is two and a half CPUs kept busy.
	CPU float64
	// RSSMax is the largest resident set seen, in bytes.
	RSSMax uint64
}

// sampler measures a process from start to stop: its CPU from its CPU time
// at both ends, and its memory every interval in between.
type sampler struct {
	proc  *process.Process
	begin time.Time
	cpu0  float64
	stop  chan struct{}
	done  sync.WaitGroup
	rss   uint64
}

func cpuTime(p *process.Process) float64 {
	t, err := p.Times()
	if err != nil {
		return -1
	}
	return t.User + t.System
}

// startSampler starts measuring pid, or returns nil when it cannot be read;
// a nil sampler's stop reports nothing.
func startSampler(pid int, interval time.Duration) *sampler {
	if pid <= 0 {
		return nil
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil
	}
	s := &sampler{proc: p, begin: time.Now(), cpu0: cpuTime(p), stop: make(chan struct{})}
	if s.cpu0 < 0 {
		return nil
	}
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
				s.sampleRSS()
			}
		}
	}()
	return s
}

func (s *sampler) sampleRSS() {
	if m, err := s.proc.MemoryInfo(); err == nil && m.RSS > s.rss {
		s.rss = m.RSS
	}
}

func (s *sampler) finish() Usage {
	if s == nil {
		return Usage{}
	}
	close(s.stop)
	s.done.Wait()
	s.sampleRSS()
	u := Usage{RSSMax: s.rss}
	if cpu1 := cpuTime(s.proc); cpu1 >= 0 {
		if wall := time.Since(s.begin).Seconds(); wall > 0 {
			u.CPU = (cpu1 - s.cpu0) / wall * 100
		}
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
