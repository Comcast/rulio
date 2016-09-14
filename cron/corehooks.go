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
	"errors"
	"fmt"
	"rulio/core"
)

func getSchedule(ctx *core.Context, fact core.Map) (string, error) {
	r, is := fact["rule"]
	if !is {
		return "", nil
	}

	rule, ok := r.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("rule %#v isn't a map", r)
	}

	schedule, scheduled := rule["schedule"]
	if !scheduled {
		return "", nil
	}

	scheduleStr, ok := schedule.(string)
	if !ok {
		return "", fmt.Errorf("schedule %#v isn't a string", schedule)
	}

	return scheduleStr, nil
}

func AddHooks(ctx *core.Context, cronner Cronner, state core.State) error {

	add := func(ctx *core.Context, state core.State, id string, fact core.Map, loading bool) error {

		if cronner.Persistent() && loading {
			return nil
		}

		schedule, err := getSchedule(ctx, fact)
		if err != nil {
			return err
		}

		if schedule == "" {
			return nil
		}

		if cronner == nil {
			return errors.New("no cron available")
		}

		location := ctx.Location().Name

		core.Log(core.INFO|CRON, ctx, "addHook", "id", id, "location", location, "schedule", schedule)

		event := fmt.Sprintf(`{"trigger!":"%s"}`, id)

		se := &ScheduledEvent{
			Id:       id,
			Event:    event,
			Schedule: schedule,
		}

		if err = cronner.ScheduleEvent(ctx, se); err != nil {
			core.Log(core.WARN|CRON, ctx, "addHook", "id", id, "error", err)
			return err
		}

		return nil
	}

	state.AddHook(add)

	rem := func(ctx *core.Context, state core.State, id string) error {
		core.Log(core.INFO|CRON, ctx, "remHook", "id", id)

		// Sad that we have to get the whole fact.

		// Yikes!  The caller of this hook already has the state lock!
		fact, err := state.Get(ctx, id)
		if err != nil {
			return err
		}
		if fact == nil {
			core.Log(core.WARN|CRON, ctx, "remHook", "missing", id)
			return nil
		}

		schedule, err := getSchedule(ctx, fact)
		if err != nil {
			return err
		}
		if schedule == "" {
			return nil
		}

		if cronner == nil {
			return errors.New("no cron available")
		}

		_, err = cronner.Rem(ctx, id)

		return err
	}
	state.RemHook(rem)

	return nil
}
