package runner

import (
	"testing"
	"time"
)

func TestBucketsAreContiguousAndOrdered(t *testing.T) {
	prev := -1
	for _, v := range []uint64{0, 1, 63, 64, 127, 128, 129, 255, 256, 1000, 1 << 20, 1<<40 + 12345, 1<<63 - 1, 1 << 63, ^uint64(0)} {
		b := bucketOf(v)
		if b < prev || b >= numBuckets {
			t.Fatalf("bucketOf(%d) = %d after %d (of %d)", v, b, prev, numBuckets)
		}
		prev = b
	}
	for v := uint64(0); v < 1<<16; v++ {
		b := bucketOf(v)
		if next := bucketOf(v + 1); next != b && next != b+1 {
			t.Fatalf("bucketOf(%d) = %d, bucketOf(%d) = %d: a bucket was skipped", v, b, v+1, next)
		}
		mid := bucketValue(b)
		if diff := int64(mid) - int64(v); diff < -int64(v)/subCount-1 || diff > int64(v)/subCount+1 {
			t.Fatalf("bucketValue(bucketOf(%d)) = %d: more than one bucket width off", v, mid)
		}
	}
}

func TestSummary(t *testing.T) {
	h := newHistogram(4)
	for i := 1; i <= 1000; i++ {
		h.shard(i).record(time.Duration(i)*time.Microsecond, false)
	}
	h.shard(0).record(time.Second, true)
	total, failed, lat := h.summary()
	if total != 1001 || failed != 1 {
		t.Fatalf("total %d failed %d, want 1001 and 1", total, failed)
	}
	near := func(name string, got, want time.Duration) {
		t.Helper()
		if d := got - want; d < -want/50 || d > want/50 {
			t.Errorf("%s = %v, want about %v", name, got, want)
		}
	}
	near("avg", lat.Avg, 500500*time.Nanosecond)
	near("tp50", lat.TP50, 500*time.Microsecond)
	near("tp75", lat.TP75, 750*time.Microsecond)
	near("tp90", lat.TP90, 900*time.Microsecond)
	near("tp95", lat.TP95, 950*time.Microsecond)
	near("tp99", lat.TP99, 990*time.Microsecond)
	near("tp999", lat.TP999, 999*time.Microsecond)
	if lat.Min != time.Microsecond || lat.Max != 1000*time.Microsecond {
		t.Errorf("min, max = %v, %v, want 1µs, 1ms", lat.Min, lat.Max)
	}
}
