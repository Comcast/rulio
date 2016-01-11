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

// These timers measure time.  A bad name.  They are supposed to be
// simple and fast.  See 'timers_test.go' for examples.

import (
	"math/rand"
	"sync"
	"time"
)

type Timer struct {
	Ctx     *Context
	Id      int64
	S       string
	Then    int64
	Elapsed int64
	Paused  bool
}

var NoTimer = Timer{nil, 0, "ignore", 0, 0, false}

// NewTimer makes a new timer with the given name.
//
// Ctx is optional.  If provided and if its system's configuration
// 'Timing' parameter is false, the a no-op timer will be returned.
func NewTimer(ctx *Context, s string) *Timer {
	if ctx != nil && ctx.GetLoc() != nil {
		c := ctx.GetLoc().Control()
		if c != nil && c.NoTiming {
			return &NoTimer
		}
	}
	t := Timer{ctx, rand.Int63(), s, time.Now().UTC().UnixNano(), 0, false}
	return &t
}

// Elapsed computes the elapsed time in nanoseconds.
//
// This method does not change the timer state.
func (t *Timer) Elapse() int64 {
	if t == &NoTimer {
		return 0
	}
	if t.Paused {
		return t.Elapsed
	}
	return t.Elapsed + (time.Now().UTC().UnixNano() - t.Then)
}

// Resume restarts a paused timer.
func (t *Timer) Resume() {
	if t == &NoTimer {
		return
	}
	t.Then = time.Now().UTC().UnixNano()
	t.Paused = false
}

// Reset zeros the current elapsed time and resets the current time.
func (t *Timer) Reset() {
	if t == &NoTimer {
		return
	}
	t.Then = time.Now().UTC().UnixNano()
	t.Elapsed = 0
}

// Pause stops the clock.
func (t *Timer) Pause() {
	if t.Paused {
		return
	}
	t.Elapsed = t.Elapse()
	t.Then = time.Now().UTC().UnixNano()
	t.Paused = true
}

// Stop computes the elapsed time (in nanosecs) and stores it in the history.
func (t *Timer) Stop() int64 {
	if t == &NoTimer {
		return 0
	}
	elapsed := t.Elapse()
	t.Elapsed = elapsed
	t.Then = time.Now().UTC().UnixNano()
	t.store(t.S)
	t.Elapsed = 0
	ms := elapsed / 1000000
	Log(TIMER, t.Ctx, "Timer.Stop", "timer", t.S, "elapsed", elapsed, "ms", ms)
	// Watch out for lots of metrics, which could generate lots of cost.
	Point(t.Ctx, "", "Timer"+t.S, elapsed/1000000, "Milliseconds")
	Point(t.Ctx, "", "Count"+t.S, 1, "Count")
	if SystemParameters.TimerWarningLimit < time.Duration(elapsed) {
		Log(WARN, t.Ctx, "Timer.Stop", "timer", t.S, "elapsed", elapsed, "warning", "slow")
	}
	return elapsed
}

// StopTag computes the elapsed time (in nanosecs) and stores it in
// the history.
//
// This method also logs (level -1) the elasped time with the given
// tag.
func (t *Timer) StopTag(tag string) int64 {
	if t == &NoTimer {
		return 0
	}
	elapsed := t.Elapse()
	t.Elapsed = elapsed
	t.Then = time.Now().UTC().UnixNano()
	t.Elapsed = elapsed
	t.store(t.S + "_" + tag)
	t.store(t.S)
	t.Elapsed = 0
	Log(TIMER, t.Ctx, "Timer.StopTag", "timer", t.S, "elapsed", elapsed, "timertag", tag)
	// Watch out for lots of metrics, which could generate lots of cost.
	Point(t.Ctx, "", "Timer"+t.S, elapsed/1000000, "Milliseconds")
	Point(t.Ctx, "", "Count"+t.S, 1, "Count")
	return elapsed
}

// In-memory timer histories

// TimerHistory stores a history for each timer.
// This history is a circular buffer.
type TimerHistory struct {
	sync.Mutex
	seq    int
	offset int
	size   int
	buffer []TimerHistoryEntry
}

func newTimerHistory() *TimerHistory {
	history := TimerHistory{}
	history.size = SystemParameters.TimerHistorySize
	history.buffer = make([]TimerHistoryEntry, history.size)
	return &history
}

// We'll need to make an atomic copy of a history for API GetTimerHistory.
func (history *TimerHistory) Copy() *TimerHistory {
	clone := TimerHistory{}
	history.Lock()
	clone.buffer = make([]TimerHistoryEntry, history.size)
	clone.offset = history.offset
	clone.seq = history.seq
	clone.size = history.size
	copy(clone.buffer, history.buffer)
	history.Unlock()
	return &clone

}

// TimerHistoryEntry stores a bit of timer state at a point in time.
type TimerHistoryEntry struct {
	// Monotonically increase sequence number.
	Seq int

	// 2014-07-03T16:12:19.902Z.  Truncated to millis because so
	// many external systems don't automatically deal with
	// nanoseconds.
	Timestamp string

	// Nanoseconds.
	Elapsed int64
}

var timerHistories = make(map[string]*TimerHistory)

// Protected by a mutex, of course.
var timersMutex = sync.Mutex{}

// ClearTimerHistories resets all timer histories.
func ClearTimerHistories() {
	Log(INFO, nil, "ClearTimerHistories")
	timersMutex.Lock()
	timerHistories = make(map[string]*TimerHistory)
	timersMutex.Unlock()
}

var tooManyTimersWarning = false

// Add to a timer's history.
func (t *Timer) store(name string) {
	if name == "" {
		name = t.S
	}

	// Get or create the history.
	timersMutex.Lock()
	history, have := timerHistories[name]
	if !have {
		if SystemParameters.MaxTimers <= len(timerHistories) {
			if !tooManyTimersWarning {
				Log(WARN, t.Ctx, "Timer.store", "warning", "Too many timers")
				// Hope bool access is atomic enough.
				tooManyTimersWarning = true
			}
		} else {
			history = newTimerHistory()
			timerHistories[name] = history
		}
	}
	timersMutex.Unlock()
	if history == nil {
		return
	}

	history.Lock()
	seq := history.seq
	entry := TimerHistoryEntry{seq, NanoStringToMilliString(NowString()), t.Elapsed}
	history.seq++
	history.buffer[history.offset] = entry
	history.offset++
	if history.offset == len(history.buffer) {
		history.offset = 0
	}
	history.Unlock()
}

// GetTimerHistory gets a history for timers with the given name.
// 'after' is sequence number corresponding to 'Seq' in entries you've
// previously gotten.  Use -1 to start.  'limit' does what you'd think.
func GetTimerHistory(name string, after int, limit int) []TimerHistoryEntry {
	timersMutex.Lock()
	history, have := timerHistories[name]
	timersMutex.Unlock()
	if !have {
		none := make([]TimerHistoryEntry, 0, 0)
		return none
	}

	clone := history.Copy()

	n := clone.size
	entries := make([]TimerHistoryEntry, 0, n)
	for i := clone.offset; i < n; i++ {
		entry := clone.buffer[i]
		if entry.Timestamp != "" && after < entry.Seq {
			entries = append(entries, entry)
		}
	}
	for i := 0; i < clone.offset; i++ {
		entry := clone.buffer[i]
		if entry.Timestamp != "" && after < entry.Seq {
			entries = append(entries, entry)
		}
	}

	drop := len(entries) - limit
	if 0 <= limit && 0 < drop {
		entries = entries[drop:]
	}
	return entries
}

// GetTimerNames returns a list of all know timer names.
func GetTimerNames() []string {
	timersMutex.Lock()
	names := make([]string, 0, len(timerHistories))
	for name, _ := range timerHistories {
		names = append(names, name)
	}
	timersMutex.Unlock()
	return names
}
