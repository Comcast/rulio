package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

type Latencies struct {
	Ns     []int
	sorted bool
	mean   float64
}

func NewLatencies(size int) *Latencies {
	ns := &Latencies{Ns: make([]int, 0, size)}
	ns.sorted = true
	return ns
}

func (ns *Latencies) Count() int {
	return len(ns.Ns)
}

func (ns *Latencies) Stable() bool {
	// ToDo: Consider Go's benchmarking heuristic.
	return false
}

func (ns *Latencies) Add(n int) *Latencies {
	if ns == nil {
		return ns
	}
	ns.Ns = append(ns.Ns, n)
	ns.sorted = false
	return ns
}

func (ns *Latencies) AddElapsed(then time.Time) *Latencies {
	n := time.Now().Sub(then).Nanoseconds()
	return ns.Add(int(n))
}

func (ns *Latencies) Mean() (int, error) {
	return ns.Percentile(50.0)
}

func (ns *Latencies) Worst() (int, error) {
	return ns.Percentile(100.0)
}

func (ns *Latencies) Percentile(p float64) (int, error) {
	ns.Sort()
	n := len(ns.Ns)
	if n == 0 {
		return 0, errors.New("no data")
	}
	i := int(p / 100.0 * float64(n-1))
	if i < 0 {
		i = 0
	}
	if n <= i {
		i = n - 1
	}
	return ns.Ns[i], nil
}

func (ns *Latencies) Sort() *Latencies {
	if !ns.sorted {
		sort.IntSlice(ns.Ns).Sort()
		ns.sorted = true
	}
	return ns
}

func (ns *Latencies) Stddev() (float64, error) {
	data := ns.Ns

	// http://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Online_algorithm

	n := 0.0
	mean := 0.0
	M2 := 0.0

	for _, x := range data {
		n = n + 1
		delta := float64(x) - mean
		mean = mean + delta/n
		M2 = M2 + delta*(float64(x)-mean)
	}

	if n < 2 {
		return 0.0, nil
	}

	variance := M2 / (n - 1)

	return math.Sqrt(variance), nil
}

type LatencyStats struct {
	Points int
	Mean   int
	P90    int
	P95    int
	P99    int
	Worst  int
	Stddev float64
}

func LatencyStatsHeader() string {
	return "points,mean,p90,p95,p99,worst,stddev"
}

func (ns *LatencyStats) CSVDiv(by int) string {
	return fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d",
		ns.Points,
		ns.Mean/by,
		ns.P90/by,
		ns.P95/by,
		ns.P99/by,
		ns.Worst/by,
		int(ns.Stddev)/by)
}

func (ns *LatencyStats) CSV() string {
	return ns.CSVDiv(1)
}

func (ns *Latencies) Stats() (*LatencyStats, error) {
	s := LatencyStats{}
	if ns == nil {
		return &s, nil
	}
	s.Points = len(ns.Ns)
	var err error
	if s.Mean, err = ns.Mean(); err != nil {
		return nil, err
	}
	if s.P90, err = ns.Percentile(90.0); err != nil {
		return nil, err
	}
	if s.P95, err = ns.Percentile(95.0); err != nil {
		return nil, err
	}
	if s.P99, err = ns.Percentile(99.0); err != nil {
		return nil, err
	}
	if s.Worst, err = ns.Worst(); err != nil {
		return nil, err
	}
	if s.Stddev, err = ns.Stddev(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (ns *Latencies) CSV() string {
	s, err := ns.Stats()
	if err != nil {
		s = &LatencyStats{}
	}
	return s.CSV()
}
