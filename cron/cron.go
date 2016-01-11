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

package cron

// The "internal" cron: A sketchy implementation of an in-memory cron
// system.
//
// This implementation is intended to support small Cron instances.
// In the context of the Rules Core, each location SHOULD have its own
// Cron instance.  Why?  Several reasons:
//
// 1. We'll rely on the Go scheduler to help with fairness.  Hopefully
// a few greedy locations won't crowd out other locations.
//
// 2. Our Rem() method does a linear scan over pending jobs (instead
// of using a map).  Don't want too many jobs.
//
// 3. We can do efficient location-specific cron crontrol (pause,
// resume, size limit).
//
// 4. Maybe problem containment.  Perhaps a problematic Cron instance
// won't always create problems for other cron instances.
//
// However, this implemention does provide some coarse yet efficient
// global control via CronBroadcasters.  The default
// SysCronBroadcaster can be used to suspend and resume all cron
// processing across all locations.

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Comcast/rulio/core"

	"github.com/gorhill/cronexpr"
)

// CRON is a crude logging component, which really is just the EXTERN
// catch-all from core/log.go.
var CRON = core.EXTERN

// CronBroadcaster can broadcast a signal to multiple Cron instances.
//
// The basis for the implementation was suggested by Bill.  All
// channel readers hear when the channel is closed.  Since we want to
// send more than one signal, we create another channel when the
// current one is closed.
type CronBroadcaster struct {
	sync.RWMutex
	suspended bool
	c         *chan struct{}
}

func NewCronBroadcaster() *CronBroadcaster {
	c := make(chan struct{})
	b := CronBroadcaster{sync.RWMutex{}, false, &c}
	return &b
}

// Get returns the broadcaster's current channel.
//
// Cron instances call this method after each channel close that is
// detected.
func (b *CronBroadcaster) Get() (*chan struct{}, bool) {
	b.RLock()
	c, d := b.c, b.suspended
	b.RUnlock()
	return c, d
}

// Suspend broadcasts that command to all Cron instances with this broadcaster.
func (b *CronBroadcaster) Suspend() bool {
	b.Lock()
	b.toggle()
	b.suspended = true
	b.Unlock()
	return true
}

// Resume broadcasts that command to all Cron instances with this broadcaster.
func (b *CronBroadcaster) Resume() bool {
	b.Lock()
	b.toggle()
	b.suspended = false
	b.Unlock()
	return true
}

// toggle sends the signal to all Cron instances with this broadcaster.
//
// Assumes the broacaster (write) lock is held by the caller.  Creates
// a new channel after closing the current one.  Cron instances get
// the new channel by calling Get() on the broadcaster.
func (b *CronBroadcaster) toggle() {
	// Assume lock held.
	close(*b.c)
	c := make(chan struct{})
	b.c = &c
}

// SysCronBroadcaster is a default, shared broadcaster.
var SysCronBroadcaster = NewCronBroadcaster()

// CronJob packages up the basics for a job.  In particular, we need a
// Go function that defines the work to be performed.
type CronJob struct {
	// Id should be same as the fact Id that created this job.
	Id string

	// Schedule is the cron schedule string.
	Schedule string

	// Expression is the parsed schedule.
	Expression *cronexpr.Expression

	// When this job should run.
	// If the job is a one-shot job, Expression will be nil.
	Next time.Time

	// Fn is the work to be performed.
	//
	// Argument is the scheduled time.
	Fn func(time.Time) error

	// Err holds the error returned by the last invocation of Fn.
	Err error
}

// Timeline is the time-order list of pending CronJobs.
type Timeline []*CronJob

func (tl Timeline) Len() int {
	return len(tl)
}

func (tl Timeline) Swap(i, j int) {
	tl[i], tl[j] = tl[j], tl[i]
}

func (tl Timeline) Less(i, j int) bool {
	return tl[i].Next.Before(tl[j].Next)
}

func (tl Timeline) Search(t time.Time) int {
	return sort.Search(len(tl), func(i int) bool {
		return t.Before(tl[i].Next)
	})
}

// Cron implements a little in-memory cron system.
//
// Not persistent; not fair.
type Cron struct {
	sync.Mutex
	Timeline
	control     chan string
	broadcaster *CronBroadcaster
	timer       *time.Timer

	// To check how our Timer is doing
	timerTarget time.Time

	// PauseDuration determines how long a 'pause' command pauses the processing loop.
	// (We have the suffix "Duration" to distinguish from the method.)
	PauseDuration time.Duration

	// Name is opaque; just used for logging.
	Name string

	// The approximate maximum number pending jobs.
	Limit int
}

// NewCron creates a new Cron instanced.
//
// If the given broadcaster in nil, SysCronBroadcaster is used.  Name
// is opaque; just for logging.
func NewCron(broadcaster *CronBroadcaster, pause time.Duration, name string, limit int) (*Cron, error) {
	if broadcaster == nil {
		broadcaster = SysCronBroadcaster
	}

	c := &Cron{sync.Mutex{},
		make([]*CronJob, 0, 0),
		nil,
		broadcaster,
		time.NewTimer(0 * time.Second),
		time.Now(),
		pause,
		name,
		limit}

	return c, nil
}

// Start starts up a goroutine that operates the cron system.
//
// This method returns after the goroutine is started.
//
// You need to call this method for the cron system to run any jobs.
func (c *Cron) Start(ctx *core.Context) {
	core.Log(core.INFO|CRON, ctx, "Cron.Start", "name", c.Name)
	go func() {
		err := c.start(ctx)
		// Make sure we log any trouble.
		if err != nil {
			core.Log(core.CRIT|CRON, ctx, "Cron.Start", "error", err, "name", c.Name)
		}
	}()
	// ToDo: Try to make sure we are in the select loop before returning here.
	c.Resume(ctx)
}

// PendingCount returns the number of known cron jobs.
func (c *Cron) PendingCount() int {
	c.Lock()
	n := c.Timeline.Len()
	c.Unlock()
	return n
}

// control is the low-level method for controlling the Cron instance.
//
// Gets the lock and sends the given command to the Cron's control channel.
func (c *Cron) command(ctx *core.Context, command string) error {
	core.Log(core.INFO|CRON, ctx, "Cron.command", "command", command, "name", c.Name)
	c.Lock()
	if c.control == nil {
		c.Unlock()
		return fmt.Errorf("Not started")
	}
	c.control <- command
	c.Unlock()
	return nil
}

// Kill the instance's loop forever.
//
// No going back.
func (c *Cron) Kill(ctx *core.Context) error {
	return c.command(ctx, "kill")
}

// Pause the instance for c.PauseDuration.
func (c *Cron) Pause(ctx *core.Context) error {
	return c.command(ctx, "pause")
}

// Suspend stops the instance's processing loop.
//
// Restart processing with Resume().
func (c *Cron) Suspend(ctx *core.Context) error {
	return c.command(ctx, "suspend")
}

// Resume restarts the instance's processing loop.
func (c *Cron) Resume(ctx *core.Context) error {
	return c.command(ctx, "resume")
}

// start launches the instance's processing loop.
func (c *Cron) start(ctx *core.Context) error {
	core.Log(core.INFO|CRON, ctx, "Cron.start", "name", c.Name)

	// Make a control channel.
	c.Lock()
	if c.control != nil {
		c.Unlock()
		return fmt.Errorf("Already started")
	}
	c.control = make(chan string, 10)
	c.Unlock()

	// Either a broadcast or a local command can suspend or resume the loop.
	// Might want separate state.
	suspendedLocally := false

	// We'll receive a broadcast on this channel.
	broadcast, suspendedByBroadcast := c.broadcaster.Get()
	if suspendedByBroadcast {
		suspendedLocally = true
		c.stopTimerLocked()
	}
LOOP:
	for {
		select {

		case <-(*broadcast):
			// Channel closed.  Toggle our state.
			// Get the (new) control channel since our pointer now points to a dead one.
			broadcast, suspendedByBroadcast = c.broadcaster.Get()
			if suspendedByBroadcast {
				suspendedLocally = true
				c.stopTimerLocked()
			} else if suspendedLocally {
				suspendedLocally = false
				c.resetTimerLocked()
			}

		case command := <-c.control:
			var err error
			switch command {
			case "pause":
				c.stopTimerLocked()
				time.Sleep(c.PauseDuration)
				c.resetTimerLocked()
			case "suspend":
				suspendedLocally = true
				c.stopTimerLocked()
				continue
			case "resume":
				if suspendedLocally {
					suspendedLocally = false
					c.resetTimerLocked()
				}
			case "kill":
				// Danger.  Can't restart from the control channel.
				c.stopTimerLocked()
				break LOOP
			default:
				err = fmt.Errorf("Cron %p %s unknown command '%s'", c, c.Name, command)
				core.Log(core.WARN|CRON, ctx, "Cron.start", "error", err, "name", c.Name)
				return err
			}

		case <-c.timer.C:

			// Let's check how well our timer is working.
			// delta := time.Now().Sub(c.timerTarget)

			now := time.Now()
			c.Lock()
			if 0 < len(c.Timeline) {
				job := c.Timeline[0]
				ready := !now.Before(job.Next)
				if ready {
					// Danger.  ToDo: Be more careful
					c.Timeline = c.Timeline[1:]
					go func(job *CronJob) {
						c.run(ctx, job)
					}(job)
					c.resetTimer()
				}
			}
			c.Unlock()
			// elapsed := time.Now().Sub(now)
		}
	}

	c.Lock()
	c.control = nil
	c.Unlock()
	return nil
}

// Once is a little check to see if a job should only be executed once.
//
// The test is if job.Expression is nil.
func (job *CronJob) Once() bool {
	return job.Expression == nil
}

func (c *Cron) run(ctx *core.Context, job *CronJob) {
	core.Log(core.INFO|CRON, ctx, "Cron.run", "job", *job, "name", c.Name)
	once := job.Once()
	err := job.Fn(time.Now())
	if err != nil {
		job.Err = err
	}
	if once {
	} else {
		// ToDo: Consider an error here.
		c.schedule(ctx, job, false)
	}
}

func (c *Cron) stopTimer() {
	// Assumes we have the lock.
	c.timer.Stop()
}

func (c *Cron) stopTimerLocked() {
	c.Lock()
	c.timer.Stop()
	c.Unlock()
}

func (c *Cron) resetTimer() {
	// Assumes we have the lock.
	if 0 < len(c.Timeline) {
		next := c.Timeline[0].Next
		c.timerTarget = next
		now := time.Now()
		delta := next.Sub(now)
		if delta < 0 {
			delta = 0 * time.Second
			c.timerTarget = now
		}
		c.timer.Reset(delta)
	} else {
		c.timer.Stop()
	}
}

func (c *Cron) resetTimerLocked() {
	c.Lock()
	c.resetTimer()
	c.Unlock()
}

func (c *Cron) insert(ctx *core.Context, job *CronJob) int {
	core.Log(core.INFO|CRON, ctx, "Cron.insert", "job", *job, "name", c.Name)
	// Assumes we have the lock.
	at := c.Timeline.Search(job.Next)
	if at == len(c.Timeline) {
		c.Timeline = append(c.Timeline, job)
	} else {
		c.Timeline = append(c.Timeline, nil)
		copy(c.Timeline[at+1:], c.Timeline[at:])
		c.Timeline[at] = job
	}
	c.resetTimer()
	return at
}

func (c *Cron) schedule(ctx *core.Context, job *CronJob, checkLimit bool) error {
	core.Log(core.INFO|CRON, ctx, "Cron.schedule", "job", *job, "name", c.Name)

	if job.Expression != nil {
		job.Next = job.Expression.Next(time.Now().UTC())
	}

	c.Lock()

	//remove existing job with the same id
	if _, err := c.rem(ctx, job.Id); nil != err {
		c.Unlock()
		core.Log(core.WARN|CRON, ctx, "Cron.schedule", "error", err)
		return err
	}

	var err error
	if checkLimit {
		count := len(c.Timeline)
		limit := c.Limit
		if limit <= count {
			err = fmt.Errorf("Cron %p %s capacity limit (%d) hit", c, c.Name, limit)
			core.Log(core.WARN|CRON, ctx, "Cron.schedule", "limit", limit, "error", err, "name", c.Name)
		}
	}
	if err == nil {
		c.insert(ctx, job)
	}

	c.Unlock()
	return err
}

// Add creates a new cron job.
//
// Use the ID to Rem() that job later if you want.  F is the work to
// be performed.  The first argument is the scheduled time for that
// invocation, and the second argument is true if the job is a
// one-shot job.
//
// The schedule syntax can have three forms:
//
// 1. A cron schedule string (supposedly in syntax at
// https://en.wikipedia.org/wiki/Cron).
//
// 2. "!TIME", where TIME is according to RFC3339.
//
// 3. "+DURATION", where DURATION is a Go Duration
// (http://golang.org/pkg/time/#ParseDuration).  Examples: "5s" means
// "5 seconds", "2m" means "2 minutes", and "1h" means "1 hour".
func (c *Cron) Add(ctx *core.Context, id string, schedule string, f func(t time.Time) error) error {
	core.Log(core.INFO|CRON, ctx, "Cron.Add", "id", id, "schedule", schedule, "name", c.Name)
	job := CronJob{}
	job.Id = id
	job.Schedule = schedule
	job.Fn = f
	if core.OneShotSchedule(schedule) {
		switch schedule[0:1] {
		case "!":
			t, err := time.Parse(time.RFC3339, schedule[1:])
			if err != nil {
				return err
			}
			job.Next = t
		case "+":
			d, err := time.ParseDuration(schedule[1:])
			if err != nil {
				return err
			}
			job.Next = time.Now().Add(d)
		default:
			return fmt.Errorf("bad one-shot schedule '%s'", schedule)
		}
	} else {
		expr, err := cronexpr.Parse(schedule)
		if err != nil {
			return err
		}
		job.Expression = expr
	}

	future := job.Next.Sub(time.Now())
	core.Log(core.DEBUG|CRON, ctx, "Cron.Add", "id", id, "in", future)

	return c.schedule(ctx, &job, true)
}

// Rem deletes the job with the given ID.
//
// Returns true if the job was found and false if not.
func (c *Cron) Rem(ctx *core.Context, id string) (bool, error) {
	// Linear scan or maintain and use a map?  Since we are moving
	// to a cron instance per location, we'll just scan.  That'll
	// obviously cost us a bit of runtime but we avoid map
	// overhead (including garbage, which could be significant
	// with many thousands of instances.
	c.Lock()
	found, err := c.rem(ctx, id)
	c.Unlock()

	return found, err
}

func (c *Cron) rem(ctx *core.Context, id string) (bool, error) {
	core.Log(core.INFO|CRON, ctx, "Cron.rem", "id", id, "name", c.Name)
	found := false
	for at, job := range c.Timeline {
		if job.Id == id {
			copy(c.Timeline[at:], c.Timeline[at+1:])
			c.Timeline = c.Timeline[0 : len(c.Timeline)-1]
			found = true
			break
		}
	}
	if !found {
		// log.Printf("Cron.Rem %p %s job %s not found", c, c.Name, id)
	}
	return found, nil
}
