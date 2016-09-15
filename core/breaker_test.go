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

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOutboundBreaker(t *testing.T) {
	b, err := NewOutboundBreaker(5, 65*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 50; i++ {
		closed, _ := b.Do(nil)
		fmt.Printf("%02d %s %v\n", i, b.Summary(), closed)
		time.Sleep(2 * time.Millisecond)
		switch i {
		case 8, 16, 24, 32, 40:
			time.Sleep(47 * time.Millisecond)
		case 10, 20, 30, 41:
			time.Sleep(27 * time.Millisecond)
		}
	}
}

func BenchmarkOutboundBreaker(b *testing.B) {
	limit := int64(100)
	c, err := NewOutboundBreaker(limit, 65*time.Millisecond)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		closed, _ := c.Do(nil)
		if limit < int64(i) && closed {
			b.Fatal("should have flipped")
		}
	}
}

func TestCPULoad(t *testing.T) {
	if !HaveProc() {
		t.Skip()
	}
	min1, min5, min15, err := CPULoad()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("load %f %f %f\n", min1, min5, min15)
}

func TestCPULoadProbe(t *testing.T) {
	if !HaveProc() {
		t.Skip()
	}

	load0, err := CPULoadProbe()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Millisecond)

		// The cached value should not have expired.
		load1, err := CPULoadProbe()
		if err != nil {
			t.Fatal(err)
		}

		if load0 != load1 {
			err := fmt.Errorf("loads %f != %f", load0, load1)
			t.Fatal(err)
		}
	}
}

func TestComboBreaker(t *testing.T) {
	if !HaveProc() {
		t.Skip()
	}

	loadLimit := 0.5 * float64(runtime.NumCPU())
	cpu := NewSimpleBreaker(CPULoadProbe, loadLimit)
	gos := GoroutineBreaker(10000)
	combo := NewComboBreaker(cpu, gos)
	status := combo.Status()
	if status.Error != nil {
		t.Fatal(status.Error)
	}
	fmt.Printf("combo closed: %v %f%%\n", status.Closed, status.Load)

	status = cpu.Status()
	if status.Error != nil {
		t.Fatal(status.Error)
	}
	fmt.Printf("cpu closed: %v %f%%\n", status.Closed, status.Load)

	status = gos.Status()
	if status.Error != nil {
		t.Fatal(status.Error)
	}
	fmt.Printf("gos closed: %v %f%%\n", status.Closed, status.Load)
}

func TestThrottleBasic(t *testing.T) {
	b, err := NewOutboundBreaker(3, 9*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	th, err := NewThrottle(9, 3, 2*time.Millisecond, b)
	if err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}
	did := int64(0)
	then := time.Now()
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(g int) {
			for i := 0; i < 12; i++ {
				requested := time.Now()
				err := th.Submit(func() error {
					atomic.AddInt64(&did, 1)
					delta := time.Now().Sub(requested)
					fmt.Printf("Throttled in %d for %v\n", g, delta)
					return nil
				})
				if err != nil {
					fmt.Printf("Submit error %d.%02d %v\n", g, i, err)
				}
			}
			wg.Done()
		}(g)
	}
	wg.Wait()
	elapsed := time.Now().Sub(then)
	rate := 1000.0 * 1000.0 * float64(did) / float64(elapsed.Nanoseconds())
	fmt.Printf("did %d over %v: %f per ms = %f ms per fn\n", did, elapsed, rate, 1.0/rate)
}
