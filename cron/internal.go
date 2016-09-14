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

// A wrapper around an internal cron so it can serve as a Cronner.

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"rulio/core"
)

// InternalCron just wraps an internal cron so it can be a Cronner.
type InternalCron struct {
	Cron *Cron
}

func (c *InternalCron) ScheduleEvent(ctx *core.Context, se *ScheduledEvent) error {
	sched, _, err := ParseSchedule(se.Schedule)
	if err != nil {
		return err
	}

	var event core.Map
	if err = json.Unmarshal([]byte(se.Event), &event); err != nil {
		return err
	}

	fn := func(t time.Time) error {
		loc := ctx.Location()
		if loc == nil {
			return errors.New("no location in ctx")
		}
		fr, err := loc.ProcessEvent(ctx, event)
		if err != nil {
			return err
		}
		core.Log(core.DEBUG|CRON, ctx, "InternalCron.ScheduleEvent", "findrules", *fr)
		return nil
	}
	return c.Cron.Add(ctx, se.Id, sched, fn)
}

func (c *InternalCron) Schedule(ctx *core.Context, sw *ScheduledWork) error {

	sched, _, err := ParseSchedule(sw.Schedule)
	if err != nil {
		return err
	}

	fn := func(t time.Time) error {
		buf := strings.NewReader(sw.Body)
		req, err := http.NewRequest(sw.Method, sw.URL, buf)
		id := sw.Id

		if err != nil {
			core.Log(core.WARN|CRON, ctx, "InternalCron.Schedule", "id", id, "error", err)
			return err
		}

		for header, val := range sw.Headers {
			req.Header.Set(header, val)
		}

		// ToDo: Don't use stock http.DefaultClient.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			core.Log(core.WARN|CRON, ctx, "InternalCron.Schedule", "id", id, "error", err)
			return err
		}

		_, err = ioutil.ReadAll(resp.Body)
		// ToDo: At least log what we got.
		if err != nil {
			core.Log(core.WARN|CRON, ctx, "ExternalCron.Schedule", "id", id, "error", err)
			return err
		}

		return nil
	}

	return c.Cron.Add(ctx, sw.Id, sched, fn)
}

func (c *InternalCron) Rem(ctx *core.Context, id string) (bool, error) {
	return c.Cron.Rem(ctx, id)
}

func (c *InternalCron) Persistent() bool {
	return false
}
