// Copyright 2015 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// End Copyright

package core

// Simple circuit breakers; throttles based on circuit breakers

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var Throttled = errors.New("throttled")

// Breaker is the basic interface for a simple circuit breaker.
//
// You can get status, submit work, and disable a circuit breaker.
type Breaker interface {

	// Status report the breaker load (but where 100% is
	// represented by 1.0), whether the breaker is closed (good)
	// or open (bad), whether the breaker is disabled, and any
	// error encountered getting this status.
	Status() BreakerStatus

	// Disable does what you think.
	Disable(bool)

	// Do submits work to the breaker.  The work is attempted only
	// if the breaker is closed.  If the work is attempted, any
	// resulting error is returned.
	//
	// The given function can be nil.
	Do(func() error) (attempted bool, err error)

	// When angry, count ten, before you speak; if very angry, an hundred.
	//
	//  --Thomas Jefferson
}

// BreakerStatus is used instead of return several values from Status().
type BreakerStatus struct {

	// Load is the ratio of the current count to the limit.
	Load float64

	// Closed indicates if the breaker is closed (good) or open
	// (bad).
	Closed bool

	// Disabled means that the breaker will allow everything.
	Disabled bool

	// Error is the last error encountered internally (if any).
	Error error
}

// OutboundBreaker is a very simple circuit breaker.
//
// An OutboundBreaker is used to control what this process does to
// other components or processes.  It is not really designed to
// protect the local component or process.
//
// An OutboundBreaker has a specified maximum rate, which is given by
// a count and a time.Duration.  You ask a OutboundBreaker do Do()
// some function, which the OutboundBreaker will only do if the
// current rate is below the specified maximum.
//
// An OutboundBreaker keeps a sliding state at a reasonable
// resolution.
//
// Safe to use concurrently.
type OutboundBreaker struct {
	sync.Mutex

	// limit is the caller-specify numerator of the maximum rate.
	//
	// Constructor argument.
	limit int64

	// interval is the caller-specified denominator of the maximum rate.
	//
	// Constructor argument.
	interval time.Duration

	// counts tracks hits per tick.
	//
	// The important state for the OutboundBreaker.  Each element
	// stores the count of actions during a span of time
	// associated (implicitly) with that element.  The method
	// slide() will slide entries off the end as time passes.
	counts []int64

	// updated is the last time this breaker was updated.
	//
	// We need to remember when we were updated in order to know
	// how much to slide() the 'counts' array when requested.
	updated time.Time

	// ticks is the size of the 'counts' array
	//
	// The time associated with a 'count' element is the
	// 'interval' divided by 'ticks'.  So a OutboundBreaker with 100 ticks
	// and a 1s interval will track counts on 10ms resolution.
	ticks int

	disabled bool
}

// breakerTicks is the default size of breakers' sliding windows.
//
// ToDo: Promote to a SystemParameter?
const breakerTicks = 20

// NewOutboundBreaker makes a circuit breaker that will open if the Send rate
// exceeds the given rate (limit/interval).
func NewOutboundBreaker(limit int64, interval time.Duration) (*OutboundBreaker, error) {
	return (&OutboundBreaker{}).init(limit, interval)
}

// Adjust allows a caller to change the circuit breaker's capacity.
func (b *OutboundBreaker) Adjust(limit int64, interval time.Duration) error {
	b.Lock()
	_, err := b.init(limit, interval)
	b.Unlock()
	return err
}

func (b *OutboundBreaker) Disable(disabled bool) {
	b.Lock()
	b.disabled = disabled
	b.Unlock()
}

// Do will execute the given thunk only if the circuit breaker is open.
//
// Returns whether the function executed was attempted and, if it was,
// any error generated.
//
// The function is executed after the OutboundBreaker's lock has been
// released.
func (b *OutboundBreaker) Do(f func() error) (bool, error) {
	b.Lock()
	now := time.Now()
	b.slide(now)
	total := int64(0)
	for _, count := range b.counts {
		total += count
	}
	closed := total < b.limit
	// log.Printf("OutboundBreaker total %d %v", total, closed)
	if closed {
		b.counts[0]++
	}
	b.Unlock()
	var err error
	if closed && f != nil {
		err = f()
	}
	return closed, err
}

// Zap hits the circuit breaker and returns whether the breaker is
// then closed (good: true) or open (bad: false).
//
// If the breaker is open when zapped, that zap will not add any load
// to the breaker.
func (b *OutboundBreaker) Zap() bool {
	closed, _ := b.Do(func() error {
		return nil
	})
	// log.Printf("OutboundBreaker.Zap %v", closed)
	return closed
}

// Summary gives a one-line summary of the breaker's state.
func (b *OutboundBreaker) Summary() string {
	b.Lock()
	b.slide(time.Now())
	total := int64(0)
	for _, count := range b.counts {
		total += count
	}
	s := fmt.Sprintf(`OutboundBreaker{total:%d, interval:"%v",counts:%#v}`,
		total, b.interval, b.counts)
	b.Unlock()
	return s
}

// Rate reports the load (1.0 = 100%), current count, the breaker's
// interval, and whether the breaker is closed (good: true) or open
// (bad: false).
func (b *OutboundBreaker) Status() BreakerStatus {
	b.Lock()
	b.slide(time.Now())
	total := int64(0)
	for _, count := range b.counts {
		total += count
	}
	load := float64(total) / float64(b.limit)
	closed := total < b.limit
	b.Unlock()
	return BreakerStatus{Load: load, Closed: closed, Disabled: false, Error: nil}
}

// Reset clears the breaker's state (but does not change its capacity).
func (b *OutboundBreaker) Reset() {
	b.Lock()
	b.updated = time.Now()
	for i, _ := range b.counts {
		b.counts[i] = 0
	}
	b.Unlock()
}

// init initializes the breaker's state.
//
// Mostly just allocates the count array.
func (b *OutboundBreaker) init(limit int64, interval time.Duration) (*OutboundBreaker, error) {
	if limit < 1 {
		return nil, fmt.Errorf("bad limit %d", limit)
	}
	ticks := breakerTicks
	b.limit = limit
	b.interval = interval
	b.ticks = ticks
	b.counts = make([]int64, ticks)
	return b, nil
}

// slide moves the count entries down the line based on the current time.
func (b *OutboundBreaker) slide(now time.Time) {
	// Assumes lock
	ns := now.Sub(b.updated).Nanoseconds()
	resolution := b.interval.Nanoseconds() / int64(b.ticks)
	ticks := int(ns / int64(resolution))
	if len(b.counts) < ticks {
		ticks = len(b.counts)
	}
	copy(b.counts[ticks:], b.counts)
	for i := 0; i < ticks; i++ {
		b.counts[i] = 0
	}
	b.updated = now
}

// ComboBreaker is a bunch of Breakers considered as one.
//
// A ComboBreaker's status is based on the worst of its constituent
// breaker's statuses.
type ComboBreaker struct {
	sync.Mutex
	bs       []Breaker
	disabled bool
}

func NewComboBreaker(bs ...Breaker) *ComboBreaker {
	return &ComboBreaker{bs: bs}
}

func (c *ComboBreaker) Disable(disabled bool) {
	c.Lock()
	c.disabled = disabled
	c.Unlock()
}

func (c *ComboBreaker) Status() BreakerStatus {
	c.Lock()
	disabled := c.disabled
	c.Unlock()

	var max float64
	closed := true
	var err error
	for _, b := range c.bs {
		status := b.Status()
		if err == nil && status.Error != nil {
			// Remember the first non-nil error.
			err = status.Error
		}
		if max < status.Load {
			max = status.Load
		}
		if !status.Closed && !status.Disabled {
			// If any breaker is open, then the combo is
			// open.
			closed = false
		}
	}
	return BreakerStatus{Load: max, Closed: closed, Disabled: disabled, Error: err}
}

func (c *ComboBreaker) Do(f func() error) (bool, error) {
	c.Lock()
	disabled := c.disabled
	c.Unlock()

	if disabled {
		if f != nil {
			return true, f()
		}
		return true, nil
	}

	check := func() error {
		return nil
	}
	for _, b := range c.bs {
		attempted, _ := b.Do(check)
		if !attempted {
			return false, nil
		}
	}

	if f != nil {
		return true, f()
	}
	return true, nil
}

// SimpleBreaker is a breaker based on some function that returns a float64.
type SimpleBreaker struct {
	sync.Mutex
	probe    func() (float64, error)
	limit    float64
	disabled bool
}

func NewSimpleBreaker(probe func() (float64, error), limit float64) *SimpleBreaker {
	return &SimpleBreaker{probe: probe, limit: limit}
}

func (c *SimpleBreaker) Disable(disabled bool) {
	c.Lock()
	c.disabled = disabled
	c.Unlock()
}

func (c *SimpleBreaker) Status() BreakerStatus {
	x, err := c.probe()
	closed := false
	if err == nil {
		closed = x < c.limit
	}
	load := x / c.limit
	c.Lock()
	disabled := c.disabled
	c.Unlock()
	return BreakerStatus{Load: load, Closed: closed, Disabled: disabled, Error: err}
}

func (c *SimpleBreaker) Do(f func() error) (bool, error) {
	status := c.Status()
	if status.Error != nil {
		return status.Closed, status.Error
	}
	if status.Error != nil {
		return status.Closed, status.Error
	}
	var err error
	if status.Closed || status.Disabled {
		if f != nil {
			err = f()
		}
	}
	return status.Closed, err
}

// GoroutineBreaker makes a SimpleBreaker based on goroutine count.
func GoroutineBreaker(limit int) *SimpleBreaker {
	return NewSimpleBreaker(func() (float64, error) {
		return float64(runtime.NumGoroutine()), nil
	}, float64(limit))
}

// ProbeTTL wraps a TTL cache around the given function.
func ProbeTTL(probe func() (float64, error), ttl time.Duration) func() (float64, error) {
	state := struct {
		sync.Mutex
		updated time.Time
		x       float64
		err     error
	}{}

	return func() (float64, error) {
		now := time.Now()
		state.Lock()
		x := state.x
		err := state.err
		if ttl < now.Sub(state.updated) {
			// Expired
			x, err = probe()
			state.x = x
			state.err = err
		}
		state.Unlock()
		return x, err
	}
}

// CPULoad gets load averages from '/proc/loadavg'.
func CPULoad() (min1 float64, min5 float64, min15 float64, err error) {
	// Also see https://github.com/c9s/goprocinfo

	bs, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}

	lines := strings.Split(string(bs), "\n")
	if len(lines) == 0 {
		err = fmt.Errorf("bad /proc/loadavg: no lines")
		return
	}
	line := lines[0]
	loads := strings.Split(line, " ")
	if len(loads) != 5 {
		err = fmt.Errorf("bad /proc/loadavg: %d fields", len(loads))
		return
	}

	if min1, err = strconv.ParseFloat(loads[0], 64); err != nil {
		return
	}
	if min5, err = strconv.ParseFloat(loads[1], 64); err != nil {
		return
	}
	if min15, err = strconv.ParseFloat(loads[2], 64); err != nil {
		return
	}

	return
}

// CPULoadProbe is a CPULoad probe for the 1-minute load average with a 1s TTL cache.
var CPULoadProbe = ProbeTTL(func() (float64, error) {
	min1, _, _, err := CPULoad()
	return min1, err
}, 1*time.Second)

// Throttle is kind of a dumb function execution throttler based on a
// OutboundBreaker.
type Throttle struct {
	sync.Mutex

	Breaker
	// pause specifies how long to wait between trying the breaker.
	pause time.Duration

	// attempts specifies how many times to poll the breaker.
	attempts int

	// pendingLimit is the maximum number of pending throttling submits.
	pendingLimit int

	// pending is the current number of pending throttling submits.
	pending int

	disabled bool
}

// NewThrottle creates a new Throttle based on an embedded Breaker.
//
// 'pause' specifies how long to wait between trying the breaker.
//
// 'attempts' specifies how many times to poll the breaker.
//
// 'pendingLimit' is the maximum number of pending throttling submits.
//
// 'pending' is the current number of pending throttling submits.
//
// You Submit() work to a Throttle, and the Throttle will do that work
// as soon as (more or less) the current rate is below the Throttle's
// Breaker's maximum rate.
func NewThrottle(attempts int, pendingLimit int, pause time.Duration, b Breaker) (*Throttle, error) {
	return &Throttle{Breaker: b,
		pause:        pause,
		attempts:     attempts,
		pendingLimit: pendingLimit,
	}, nil
}

func (t *Throttle) Disable(disabled bool) {
	t.Lock()
	t.disabled = disabled
	t.Unlock()
}

// Pending reports how many throttle submissions are pending.
//
// Also returns the load (relative to the maximum pending
// submissions).
func (t *Throttle) Pending() (int, float64) {
	t.Lock()
	pending := t.pending
	pendingLimit := t.pendingLimit
	t.Unlock()
	return pending, float64(pending) / float64(pendingLimit)
}

// Modify allows you to change the Throttle's specifications.
//
// You can also change the embedded Breaker using 'Adjust()'.
func (t *Throttle) Modify(pause time.Duration, attempts int) {
	t.Lock()
	t.pause = pause
	t.attempts = attempts
	t.Unlock()
}

// ThrottleExhausted is a non-fatal condition that occurs if
// submission used all of its attempts without success.
var ThrottleExhausted = &Condition{"throttle attempts exhausted", "ephemeral"}

// ThrottleOverflow is a non-fatal condition that occurs when a
// submission would result in too many pending submissions.
var ThrottleOverflow = &Condition{"throttle overflow", "ephemeral"}

// Submit sends a thunk to the throttle.
//
// The throttle will execute the thunk only if the embedded Breaker
// says it can.  If the function is executed, it's error (if any) is
// returned.  This function can also return ThrottleExhausted or
// ThrottleOverflow.
//
// The given function should not panic even if the caller tries to
// recover.
func (t *Throttle) Submit(f func() error) error {

	// Once is an accident. Twice is coincidence. Three times is
	// an enemy action.
	//
	//   --Ian Fleming in Goldfinger

	t.Lock()
	attempts := t.attempts
	pause := t.pause
	pendingLimit := t.pendingLimit
	pending := t.pending
	tooMany := pendingLimit < pending
	disabled := t.disabled
	if !tooMany || disabled {
		t.pending++
	}
	t.Unlock()
	if tooMany {
		return ThrottleOverflow
	}

	var err error
	var worked bool
	for i := 0; i < attempts; i++ {
		// Danger: If the given thunk panics, then the
		// t.pending decrement below will not execute.  ToDo:
		// Maybe pay the price of a defer to avoid this risk.
		worked, err = t.Do(f)
		if worked {
			break
		}
		// Did not attempt to execute the thunk, so pause and
		// then retry.
		time.Sleep(pause)
	}
	t.Lock()
	t.pending--
	t.Unlock()

	if worked {
		return err
	}

	return ThrottleExhausted
}

func HaveProc() bool {
	_, err := os.Stat("/proc")
	if err != nil {
		// Ever tried. Ever failed.
		// No matter. Try Again.
		// Fail again. Fail better.
		//
		// --Samuel Beckett
		return false
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}
