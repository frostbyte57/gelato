package store

import "time"

type RollingCounter struct {
	buckets []uint64
	start   time.Time
	step    time.Duration
}

func NewRollingCounter(bucketCount int, step time.Duration, now time.Time) *RollingCounter {
	if bucketCount <= 0 {
		bucketCount = 1
	}
	return &RollingCounter{
		buckets: make([]uint64, bucketCount),
		start:   now,
		step:    step,
	}
}

func (r *RollingCounter) Add(count uint64, now time.Time) {
	r.advance(now)
	idx := r.index(now)
	r.buckets[idx] += count
}

func (r *RollingCounter) Snapshot(now time.Time) []uint64 {
	r.advance(now)
	out := make([]uint64, len(r.buckets))
	copy(out, r.buckets)
	return out
}

func (r *RollingCounter) advance(now time.Time) {
	if len(r.buckets) == 0 {
		return
	}
	if now.Before(r.start) {
		return
	}
	elapsed := now.Sub(r.start)
	steps := int(elapsed / r.step)
	if steps <= 0 {
		return
	}
	if steps >= len(r.buckets) {
		for i := range r.buckets {
			r.buckets[i] = 0
		}
		r.start = now
		return
	}
	for i := 0; i < steps; i++ {
		idx := (r.index(r.start.Add(time.Duration(i)*r.step)) + 1) % len(r.buckets)
		r.buckets[idx] = 0
	}
	r.start = r.start.Add(time.Duration(steps) * r.step)
}

func (r *RollingCounter) index(t time.Time) int {
	if len(r.buckets) == 0 {
		return 0
	}
	elapsed := t.Sub(r.start)
	return int(elapsed/r.step) % len(r.buckets)
}
