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

// Make Crolt into a Cronner for the simple engine API.
//
// See ../crolt, which will need to be running for this code to function.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Comcast/rulio/core"
)

type CroltSimple struct {
	// CroltURL points to the cron service.
	CroltURL string

	// RulesURL points to a rules endpoint that speaks the
	// primitive API.
	RulesURL string
}

// CroltJob is the structure of a Crolt job.
//
// We could use the actual Crolt type.  Instead, we pretend that we
// don't have access to it.
type CroltJob struct {
	Account  string            `json:"account"`
	Id       string            `json:"id"`
	URL      string            `json:"url"`
	Schedule string            `json:"schedule"`
	Method   string            `json:"method"`
	Header   map[string]string `json:"header"`
	Body     string            `json:"requestBody"`
}

// APIProcessEventRequest helps to serialize a request for the simple
// ProcessEvent HTTP API.
type APIProcessEventRequest struct {
	URI      string      `json:"uri"`
	Location string      `json:"location"`
	Event    interface{} `json:"event"`
}

// ScheduleEvent packages up a ProcessEvent call for the simple rules
// HTTP API.
func (c *CroltSimple) ScheduleEvent(ctx *core.Context, se *ScheduledEvent) error {
	core.Log(core.INFO|CRON, ctx, "CroltSimple.ScheduleEvent", "se", *se)

	if ctx.Location() == nil {
		return errors.New("no location in ctx")
	}

	// Yikes.  Have to parse se.Event?

	var event interface{}
	err := json.Unmarshal([]byte(se.Event), &event)
	if err != nil {
		return err
	}

	req := APIProcessEventRequest{
		URI:      "/loc/events/ingest",
		Location: ctx.Location().Name,
		Event:    event,
	}

	js, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	work := Work{
		URL:    fmt.Sprintf("%s/json", strings.TrimRight(c.RulesURL, "/")),
		Method: "POST",
		Body:   string(js),
	}

	ctl := WorkControl{
		Schedule: se.Schedule,
		Tag:      ctx.Location().Name,
		Id:       se.Id,
	}

	sw := &ScheduledWork{work, ctl}

	return c.Schedule(ctx, sw)
}

// Schedule makes the Crolt request to create a new job.
func (c *CroltSimple) Schedule(ctx *core.Context, work *ScheduledWork) error {
	core.Log(core.INFO|CRON, ctx, "CroltSimple.Schedule", "work", *work)
	id := work.Id

	if work.Headers != nil {
		return errors.New("this external cron doesn't (yet?) support headers")
	}

	job := CroltJob{
		Schedule: work.Schedule,
		Account:  work.Tag,
		Id:       id,
		URL:      work.URL,
		Method:   work.Method,
		Body:     work.Body,
	}

	if nil != ctx && nil != ctx.App {
		job.Header = ctx.App.GenerateHeaders(ctx)
	}

	js, err := json.Marshal(job)
	if err != nil {
		core.Log(core.WARN|CRON, ctx, "CroltSimple.Schedule", "id", id, "error", err)
		return err
	}
	core.Log(core.INFO|CRON, ctx, "CroltSimple.Schedule", "body", string(js))

	body := string(js)
	url := strings.Trim(c.CroltURL, "/") + "/add"
	req := core.NewHTTPRequest(ctx, "POST", url, body)

	_, err = req.Do(ctx)
	if nil != err {
		core.Log(core.WARN|CRON, ctx, "CroltSimple.Schedule", "id", id, "error", err)
		return err
	}

	return nil
}

// Rem removes the job with the given id from crolt.
//
// The boolean return value isn't particularly meaningful (yet?).
func (c *CroltSimple) Rem(ctx *core.Context, id string) (bool, error) {
	core.Log(core.INFO|CRON, ctx, "CroltSimple.Rem", "id", id)

	if ctx.Location() == nil {
		return false, errors.New("no location in ctx")
	}

	url := strings.Trim(c.CroltURL, "/rem")
	url += "?account=" + ctx.Location().Name
	url += "&id=" + id
	ctx.Log(core.INFO, "Cron.Rem", "url", url)

	req := core.NewHTTPRequest(ctx, "GET", url, "")

	resp, err := req.Do(ctx)
	if nil != err {
		core.Log(core.WARN|CRON, ctx, "CroltSimple.Rem", "id", id, "error", err)
		return false, err
	}

	core.Log(core.WARN|CRON, ctx, "CroltSimple.Rem", "id", id, "status", resp.Status)
	// ToDo: Something useful with resp.Status.

	return true, nil
}

func (c *CroltSimple) Persistent() bool {
	return true
}
