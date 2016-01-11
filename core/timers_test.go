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
	"testing"
	"time"
)

func TestTimer(t *testing.T) {
	timer := NewTimer(nil, "Test")
	time.Sleep(100 * time.Millisecond)
	timer.Pause()
	time.Sleep(100 * time.Millisecond)
	timer.Resume()
	time.Sleep(100 * time.Millisecond)
	elapsed := timer.StopTag("TestTimer")
	ms := int64(1000 * 1000)
	if elapsed < 200*ms || 400*ms < elapsed {
		t.Logf("elapsed %d\n", elapsed)
		t.Fail()
	}
}

func TestTimerReset(t *testing.T) {
	timer := NewTimer(nil, "TestReset")
	time.Sleep(100 * time.Millisecond)
	timer.Reset()
	elapsed := timer.Stop()
	ms := int64(1000 * 1000)
	if 1*ms < elapsed {
		t.Logf("elapsed %d\n", elapsed)
		t.Fail()
	}
}

func TestTimerHistory(t *testing.T) {
	ClearTimerHistories()

	timer := NewTimer(nil, "TestHistory")
	time.Sleep(10 * time.Millisecond)
	timer.Stop()

	timer = NewTimer(nil, "TestHistory")
	time.Sleep(10 * time.Millisecond)
	timer.Stop()

	expected := 2
	if SystemParameters.TimerHistorySize == 0 {
		expected = 0
	}

	if len(GetTimerHistory("TestHistory", -1, 10)) != expected {
		t.Logf("history: %v", GetTimerHistory("TestHistory", 0, 10))
		t.Fail()
	}
	expected = 1
	if SystemParameters.TimerHistorySize == 0 {
		expected = 0
	}

	if len(GetTimerNames()) != expected {
		t.Logf("names: %v", GetTimerNames())
		t.Fail()
	}
}

func BenchmarkTimerStartStop(b *testing.B) {
	ctx := BenchContext("Timer")
	for i := 0; i < b.N; i++ {
		NewTimer(ctx, "TestHistory").Stop()
	}
}

func BenchmarkTimerHistory(b *testing.B) {
	ctx := BenchContext("TimerHistory")
	timerName := "BenchmarkTimerHistory"
	for i := 0; i < 10; i++ {
		NewTimer(ctx, timerName).Stop()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := len(GetTimerHistory(timerName, -1, 10))
		if 10 != n {
			b.Fatalf("unexpected %d history size", n)
		}
	}
}
