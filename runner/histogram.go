package runner

import (
	"math/bits"
	"sync/atomic"
	"time"
)

// The latency histogram is log-linear: every power of two is cut into
// subCount buckets, so a bucket is at most 1/subCount (about 1.6%) of the
// value wide, from 1ns to the largest time.Duration, in a fixed array. That
// keeps the memory of a run independent of how many calls it makes, which
// kitex-benchmark's list of every call's cost is not: at a million calls a
// second that list is 8MB a second.
const (
	subBits    = 6
	subCount   = 1 << subBits
	numBuckets = (64-subBits)<<subBits + subCount
)

func bucketOf(v uint64) int {
	if v < subCount {
		return int(v)
	}
	shift := bits.Len64(v) - subBits - 1
	return (shift+1)<<subBits + int(v>>shift) - subCount
}

// bucketValue is the middle of bucket i, which is what a percentile falling
// in it reports.
func bucketValue(i int) uint64 {
	if i < 2*subCount {
		return uint64(i)
	}
	shift := i>>subBits - 1
	low := uint64(i&(subCount-1)+subCount) << shift
	return low + (uint64(1)<<shift)/2
}

// shard is the part of the histogram a group of workers records into.
// Workers are spread over many shards so that their atomic adds rarely meet
// on a cache line.
type shard struct {
	counts [numBuckets]atomic.Uint64
	total  atomic.Uint64
	failed atomic.Uint64
	sum    atomic.Uint64
	max    atomic.Uint64
	_      [64]byte
}

func (s *shard) record(cost time.Duration, failed bool) {
	s.total.Add(1)
	if failed {
		s.failed.Add(1)
		return
	}
	v := uint64(cost)
	s.counts[bucketOf(v)].Add(1)
	s.sum.Add(v)
	for {
		m := s.max.Load()
		if v <= m || s.max.CompareAndSwap(m, v) {
			break
		}
	}
}

type histogram struct {
	shards []shard
}

func newHistogram(n int) *histogram {
	return &histogram{shards: make([]shard, n)}
}

func (h *histogram) shard(worker int) *shard {
	return &h.shards[worker%len(h.shards)]
}

// Latency is the distribution of the successful calls' round trips.
type Latency struct {
	Avg  time.Duration
	P50  time.Duration
	P90  time.Duration
	P99  time.Duration
	P999 time.Duration
	Max  time.Duration
}

// summary returns the calls made, the calls that failed and the latency of
// the rest.
func (h *histogram) summary() (total, failed uint64, lat Latency) {
	var counts [numBuckets]uint64
	var sum, max uint64
	for i := range h.shards {
		s := &h.shards[i]
		total += s.total.Load()
		failed += s.failed.Load()
		sum += s.sum.Load()
		if m := s.max.Load(); m > max {
			max = m
		}
		for b := range counts {
			counts[b] += s.counts[b].Load()
		}
	}
	ok := total - failed
	if ok == 0 {
		return total, failed, lat
	}
	lat.Avg = time.Duration(sum / ok)
	lat.Max = time.Duration(max)
	quantiles := []struct {
		q   float64
		dst *time.Duration
	}{{0.50, &lat.P50}, {0.90, &lat.P90}, {0.99, &lat.P99}, {0.999, &lat.P999}}
	var seen uint64
	next := 0
	for b, c := range counts {
		if c == 0 {
			continue
		}
		seen += c
		for next < len(quantiles) && float64(seen) >= quantiles[next].q*float64(ok) {
			v := time.Duration(bucketValue(b))
			if v > lat.Max {
				v = lat.Max
			}
			*quantiles[next].dst = v
			next++
		}
	}
	return total, failed, lat
}
