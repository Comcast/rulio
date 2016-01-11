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
	"testing"
)

func TestParseSchedule1(t *testing.T) {
	sched := "* * * * *"
	s, m, err := ParseSchedule(sched + "?random_min=4&eat=good+tacos")
	if err != nil {
		t.Fatal(err)
	}
	if s != sched {
		t.Fatal("bad schedule")
	}
	if m["eat"] != "good tacos" {
		t.Fatal("no good tacos: " + m["eat"])
	}
}

func TestParseSchedule2(t *testing.T) {
	sched := "* * * * *"
	s, _, err := ParseSchedule(sched)
	if err != nil {
		t.Fatal(err)
	}
	if s != sched {
		t.Fatal("bad schedule")
	}
}
