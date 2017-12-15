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
	"net/url"
	"strings"
)

// Work represents a basic HTTP request that an internal or external
// cron can issue as a cron job.
//
// We'll likely need to add to this struct (or another) to support
// additional HTTP request properties such as timeouts.
type Work struct {
	// URL is the target for the request.
	URL string `json:"url"`

	// Method is the HTTP method for the request.
	Method string `json:"method"`

	// Headers are added to the request headers.
	//
	// ToDo: Values should be []string, not string.
	Headers map[string]string `json:"headers"`

	// Body is the optional body for the HTTP request.
	Body string `json:"body"`
}

// WorkControl includes the schedule and perhaps an id for the job.
type WorkControl struct {
	// Schedule is a string in our extended cron syntax.
	//
	// See elsewhere (?) for that syntax.
	//
	// Basically a cron schedule, "!timestamp", or "+duration".
	Schedule string `json:"schedule"`

	// Tag is an optional label that a cron system might use to
	// facilitate access to a batch of jobs.  For example, the
	// location's name, which could enable the deletion of all
	// jobs in that location.
	Tag string `json:"tag"`

	// Id is a an optional id for the job.
	//
	// ToDo: Elaborate.
	Id string `json:"id"`
}

// ScheduledWork associates Work with its WorkControl.
type ScheduledWork struct {
	Work
	WorkControl
}

// ScheduledEvent is a scheduled rule engine event.
type ScheduledEvent struct {
	// Id is an optional transaction id.
	Id string

	// Event is a JSON representation of an event.
	Event string

	// Schedule is the schedule for the event in our extended cron
	// syntax.
	Schedule string
}

// Cronner is a generic cron service.
//
// We'll probably add some other methods later.
//
// For example, we might want a method that enables us to remove all
// jobs for a given location.  We might also want special methods for
// other engine APIs.
type Cronner interface {
	// ScheduleEvent schedules a rule engine event.
	//
	// We have this special, API-specific method due to our desire
	// to have cron functionality that does not require an HTTP
	// API.
	ScheduleEvent(ctx *core.Context, work *ScheduledEvent) error

	// Schedule is a generic scheduler for HTTP request work.
	Schedule(ctx *core.Context, work *ScheduledWork) error

	// Rem removes the job with the given id.
	//
	// The boolean return value might indicate whether the job was
	// found.  Probably isn't meaningful.
	Rem(ctx *core.Context, id string) (bool, error)

	// Persistent reports whether the Cronner's state is
	// persistent or ephemeral.
	//
	// A persistent Cronner will not get updates when Locations
	// are loaded (since such a Cronner would have gotten updates
	// when the API calls occurred).
	Persistent() bool
}

func ParseSchedule(schedule string) (string, map[string]string, error) {
	pair := strings.SplitN(strings.TrimSpace(schedule), "?", 2)
	sched := pair[0]
	props := make(map[string]string)
	if 1 < len(pair) {
		for _, pv := range strings.Split(pair[1], "&") {
			ss := strings.SplitN(pv, "=", 2)
			if len(ss) != 2 {
				err := fmt.Errorf("bad prop-value: '%s'", pv)
				return sched, props, err
			}
			p := ss[0]
			v, err := url.QueryUnescape(ss[1])
			if err != nil {
				return sched, props, err
			}
			props[p] = v
		}
	}
	return sched, props, nil
}
