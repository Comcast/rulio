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

import (
	"fmt"
	"github.com/Comcast/rulio/core"
	"log"
	"testing"
	"time"
)

func TestCron(t *testing.T) {
	ctx := core.NewContext("test")
	log.Println("TestCron")

	c, err := NewCron(nil, 1*time.Second, "test", 10)
	if err != nil {
		t.Fatal(err)
	}

	then := time.Now().Add(2 * time.Second).Format(time.RFC3339)
	log.Printf("then: %s", then)
	c.Add(ctx, "1", "!"+then, func(t time.Time) error {
		fmt.Printf("RUNNING 1 AT %v\n", t)
		return nil
	})

	c.Add(ctx, "2", "1-59/2 * * * * * *", func(t time.Time) error {
		fmt.Printf("RUNNING 2 AT %v\n", t)
		return nil
	})

	c.Add(ctx, "3", "+3s", func(t time.Time) error {
		fmt.Printf("RUNNING 3 AT %v\n", t)
		return nil
	})

	counter := 0
	last := time.Now()
	c.Add(ctx, "4", "1-59/2 * * * * * *", func(t time.Time) error {
		now := time.Now()
		delta := now.Sub(last)
		last = now
		fmt.Printf("RUNNING 4 AT %v (counter %d, elapsed %v)\n", t, counter, delta)
		counter++
		return nil
	})

	time.Sleep(6 * time.Second)
	c.Rem(ctx, "2")

	time.Sleep(6 * time.Second)

	SysCronBroadcaster.Suspend()
	time.Sleep(3 * time.Second)

	SysCronBroadcaster.Resume()
	time.Sleep(3 * time.Second)

	c.Rem(ctx, "4")
}

func TestCronResume(t *testing.T) {
	ctx := core.NewContext("test")
	log.Println("TestCron1")

	c, err := NewCron(nil, 1*time.Second, "test", 10)
	if err != nil {
		t.Fatal(err)
	}

	//create "1"
	counter := 0
	last := time.Now()
	c.Add(ctx, "1", "1-59/1 * * * * * *", func(t time.Time) error {
		now := time.Now()
		delta := now.Sub(last)
		last = now
		fmt.Printf("RUNNING 1 AT %v (counter %d, elapsed %v)\n", t, counter, delta)
		counter++
		return nil
	})

	time.Sleep(6 * time.Second)

	//suspend all
	SysCronBroadcaster.Suspend()

	c2, err := NewCron(nil, 1*time.Second, "test", 10)
	if err != nil {
		t.Fatal(err)
	}

	//create "2" which should be suspeneded
	counter2 := 0
	last2 := time.Now()
	c2.Add(ctx, "2", "1-59/1 * * * * * *", func(t time.Time) error {
		now := time.Now()
		delta := now.Sub(last2)
		last2 = now
		fmt.Printf("RUNNING 2 AT %v (counter %d, elapsed %v)\n", t, counter2, delta)
		counter2++
		return nil
	})

	time.Sleep(5 * time.Second)

	//resume only "2"
	c2.Resume(ctx)

	time.Sleep(5 * time.Second)

	//suspend all again
	SysCronBroadcaster.Suspend()

	time.Sleep(5 * time.Second)

	//resume all
	SysCronBroadcaster.Resume()
	time.Sleep(3 * time.Second)

	c.Rem(ctx, "1")
	c2.Rem(ctx, "2")
}

func TestCronUTC(t *testing.T) {
	// Attempt to test that scheduled cron jobs run on UTC time.

	ctx := core.NewContext("test")

	c, err := NewCron(nil, 1*time.Second, "test", 10)
	if err != nil {
		t.Fatal(err)
	}
	c.Start(ctx)

	// We'll listen on this channel to learn what's happened.
	ch := make(chan bool)
	then := time.Now()
	fn := func(now time.Time) error {
		fmt.Printf("CronUTC fn %v (elapsed %v)\n", now, now.Sub(then))
		// Report that this job executed.
		ch <- true
		return nil
	}

	// What time is it now?
	now := time.Now().UTC()
	hour := now.Hour()
	min := now.Minute()
	sec := now.Second()
	fmt.Printf("CronUTC %02d:%02d:%02d (%v)\n", hour, min, sec, now)

	// Make a schedule to fire 10 seconds from now.
	sec += 10
	if 60 <= sec {
		sec = 0
		min++
	}
	if 60 <= min {
		min = 0
		hour++
	}
	if 24 <= hour {
		hour = 0
	}
	// That logic will fail to catch the situation when local time
	// is ahead of UTC and the code has a certain bug.

	// This job should execute within 10 seconds.
	schedule := fmt.Sprintf("%d %d %d * * * *", sec, min, hour)
	fmt.Printf("CronUTC schedule '%s'\n", schedule)
	if err = c.Add(ctx, "test", schedule, fn); err != nil {
		t.Fatal(err)
	}

	// Wait long enough for the job to execute.  Hopefully.
	go func() {
		time.Sleep(1 * time.Minute)
		// Report that we, not the job, are responding.
		ch <- false
	}()

	got := <-ch
	if !got {
		t.Fatalf("heard %v, which wasn't from the job", got)
	}
}
