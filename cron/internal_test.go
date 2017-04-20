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
	"rulio/core"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInternalCron(t *testing.T) {
	ch := make(chan string)
	handler := func(w http.ResponseWriter, r *http.Request) {
		msg := "Handled " + r.URL.String()
		log.Println(msg)
		ch <- msg
		fmt.Fprintf(w, msg)
	}
	service := httptest.NewServer(http.HandlerFunc(handler))
	fmt.Println("Service: " + service.URL)
	defer service.Close()

	ctx := core.TestContext("InternalCron")
	c, err := NewCron(nil, time.Second, "test", 10)
	if err != nil {
		t.Fatal(err)
	}
	go c.Start(ctx)

	ic := &InternalCron{c}

	work := Work{
		URL:    service.URL,
		Method: "GET",
		Body:   "hello",
	}
	ctl := WorkControl{"+1s", "test1", "1"}
	sw := &ScheduledWork{work, ctl}

	if err = ic.Schedule(ctx, sw); err != nil {
		t.Fatal(err)
	}

	var got string
	tick := time.Tick(2 * time.Second)
	select {
	case got = <-ch:
	case <-tick:
	}

	if got == "" {
		t.Fatal("timed out")
	}
}
