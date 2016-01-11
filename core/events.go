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
	"sync"
)

var Complete = &Condition{"complete", "complete"}

type FindRules struct {
	// Event is the input event.
	Event map[string]interface{} `json:"event,omitempty"`

	// Disposition reports how things went.
	Disposition *Condition `json:"disposition,omitempty"`

	// Children are the EvalRuleConditions for each found rule.
	Children []*EvalRule `json:"children,omitempty"`

	Values []interface{} `json:"values"`
}

func (w *FindRules) Do(ctx *Context, loc *Location) {

	// Smokey, this is not 'Nam. This is bowling. There are rules.
	//
	//   --Walter in The Big Lebowski.

	Log(DEBUG, ctx, "FindRules.Do", "location", loc.Name, "work", *w)

	var rs map[string]Map
	// Look for a special property that embeds a rule.
	ruleId, given := w.Event["trigger!"]
	embedded := false
	if given {
		id, ok := ruleId.(string)
		if !ok {
			err := fmt.Errorf("ruleId %#v is not a string", ruleId)
			w.Disposition = &Condition{err.Error(), "fatal"}
			return
		}
		rule, err := loc.GetRule(ctx, id)
		if err != nil {
			w.Disposition = &Condition{err.Error(), "nonfatal"}
			return
		}
		rs = make(map[string]Map)
		rs[id] = rule
	} else {
		embed, given := w.Event["evaluate!"]
		if given {
			embedded = true
			m, ok := embed.(map[string]interface{})
			if !ok {
				err := fmt.Errorf("%#v isn't a rule", embed)
				w.Disposition = &Condition{err.Error(), "fatal"}
				return
			}
			rs = make(map[string]Map)
			rs["embedded"] = Map(m)
		} else {
			var err error
			rs, err = loc.searchRulesAncestors(ctx, w.Event)
			if err != nil {
				w.Disposition = &Condition{err.Error(), "unknown"}
				return
			}
		}
	}

	w.Children = make([]*EvalRule, 0, 0)
	for id, m := range rs {
		Log(DEBUG, ctx, "FindRules.Do", "rid", id)
		rule, err := RuleFromMap(ctx, m)
		if err != nil {
			// Will be a little odd to arrive here, but
			// could happen.  A retry is actually
			// plausible if we go through external queues,
			// which would allow the code to change.  But
			// that's a little much.  For now, we'll fail
			// without retry if anything does wrong in
			// this function.
			w.Disposition = &Condition{err.Error(), "fatal"}
			return
		}
		rule.Id = id

		var bss []Bindings
		if !embedded {
			if enabled, _ := loc.RuleEnabled(ctx, id); !enabled {
				// ToDo: Maybe not ignore error.
				continue
			}
		}

		// ToDo: Check that this rule hasn't expired (even if
		// from a trigger).

		if rule.When != nil {
			eventPattern := rule.When.Pattern
			bss, err = Matches(ctx, eventPattern, w.Event)
			if err != nil {
				w.Disposition = &Condition{err.Error(), "fatal"}
				return
			}
		} else {
			// Scheduled rule (triggered)
			bss = make([]Bindings, 0)
			bss = append(bss, make(Bindings))
		}

		if 0 < len(bss) {
			child := &EvalRule{
				Rule:      (*CleanRule)(rule),
				Bindingss: bss,
				Parent:    w,
			}
			w.Children = append(w.Children, child)
		}
	}

	w.Disposition = Complete
}

type EvalRule struct {
	// Rule is the target rule.
	Rule *CleanRule `json:"rule,omitempty"`

	Bindingss []Bindings `json:"bindingss,omitempty"`

	Disposition *Condition `json:"disposition,omitempty"`

	Children []*EvalRuleCondition `json:"children,omitempty"`

	DoneWork *RuleDone `json:",omitempty"`

	Parent *FindRules `json:"-"`
}

func (w *EvalRule) Do(ctx *Context, loc *Location) {
	Log(DEBUG, ctx, "EvalRule.Do", "location", loc.Name, "work", *w)

	w.Children = make([]*EvalRuleCondition, 0, 0)
	for _, bs := range w.Bindingss {
		child := &EvalRuleCondition{
			Bindings: bs,
			Parent:   w,
		}
		w.Children = append(w.Children, child)
	}
	w.Disposition = Complete
}

func (w *EvalRule) Dispositions() []Condition {
	conds := make([]Condition, 0, 0)
	conds = append(conds, *w.Disposition)
	for _, erc := range w.Children {
		conds = append(conds, *erc.Disposition)
		for _, era := range erc.Children {
			conds = append(conds, *era.Disposition)
		}
	}
	return conds
}

type EvalRuleCondition struct {
	// Bindingss from the event matched against the rule's 'when'
	// pattern.
	Bindings Bindings `json:"bindings,omitempty"`

	Disposition *Condition `json:"disposition,omitempty"`

	// Children are the ExecRuleActions for each Bindings.
	Children []*ExecRuleAction `json:"children,omitempty"`

	Parent *EvalRule `json:"-"`
}

func (w *EvalRuleCondition) Do(ctx *Context, loc *Location) {
	Log(DEBUG, ctx, "EvalRuleCondition.Do", "location", loc.Name, "work", *w)

	Metric(ctx, "RuleEvaluated", "location", loc.Name, "ruleId", w.Parent.Rule.Id)

	Metric(ctx, "RuleEvaluated", "location", loc.Name, "ruleId", w.Parent.Rule.Id)

	qr := InitialQueryResult(ctx)
	qr.Bss = make([]Bindings, 1)
	qr.Bss[0] = w.Bindings

	q := w.Parent.Rule.Condition.Get()
	if q != nil {
		qc := QueryContext{[]string{loc.Name}}
		qrNext, err := ExecQuery(ctx, q, loc, qc, qr)
		if err != nil {
			// We'll need more info from q.Exec() to decide for real.
			w.Disposition = &Condition{err.Error(), "unknown"}
			return
		}
		qr = *qrNext
	}

	w.Children = make([]*ExecRuleAction, 0, 0)

	if 0 < len(qr.Bss) {
		Metric(ctx, "RuleTriggered", "location", loc.Name, "ruleId", w.Parent.Rule.Id)
	}

	for _, bs := range qr.Bss {
		_, haveEventBinding := bs["?event"]
		if !haveEventBinding {
			bs["?event"] = w.Parent.Parent.Event
		}

		_, haveLocation := bs["?location"]
		if !haveLocation && loc != nil {
			bs["?location"] = loc.Name
		}

		_, haveRuleId := bs["?ruleId"]
		if !haveRuleId {
			bs["?ruleId"] = w.Parent.Rule.Id
		}

		if nil != ctx && nil != ctx.App {
			bs = ctx.App.ProcessBindings(ctx, bs)
		}

		for _, action := range w.Parent.Rule.Actions {
			child := &ExecRuleAction{
				Bindings: bs,
				Act:      Action(action),
				Parent:   w,
			}
			w.Children = append(w.Children, child)
		}
	}

	w.Disposition = Complete
}

type ExecRuleAction struct {
	// Bindings from the combined event patching and condition
	// evaluation.
	//
	// Not "Bindingss" because we execute the action for each
	// bindings in Bindingss that resulted from the rule condition
	// evaluation.
	Bindings map[string]interface{} `json:"bindings,omitempty"`

	// Act is the action to execute.
	Act Action `json:"action,omitempty"`

	Disposition *Condition `json:"disposition,omitempty"`

	Parent *EvalRuleCondition `json:"-"`

	// Value is the result of the action execution.
	Value interface{} `json:"value,omitempty"`
}

func (w *ExecRuleAction) Do(ctx *Context, loc *Location) {

	//  If you do not on every occasion refer each of your actions
	//  to the ultimate end prescribed by nature, but instead of
	//  this in the act of choice or avoidance turn to some other
	//  end, your actions will not be consistent with your
	//  theories.
	//
	//    --Epicurus

	Log(DEBUG, ctx, "ExecRuleAction.Do", "location", loc.Name, "work", *w)

	Metric(ctx, "ActionExecuted", "location", loc.Name)

	Metric(ctx, "ActionExecuted", "location", loc.Name)

	x, err := loc.ExecAction(ctx, w.Bindings, w.Act)
	if err != nil {
		w.Disposition = &Condition{err.Error(), "unknown"}
		return
	}
	w.Disposition = Complete
	w.Value = x
}

type RuleDone struct {
	Parent      *EvalRule  `json:"-"`
	Disposition *Condition `json:"disposition,omitempty"`
}

func OneShotSchedule(schedule string) bool {
	if 0 == len(schedule) {
		return false
	}
	c := schedule[0]
	return c == '+' || c == '!'
}

func (w *RuleDone) Do(ctx *Context, loc *Location) {
	Log(DEBUG, ctx, "RuleDone.Do", "location", loc.Name, "work", *w)
	if OneShotSchedule(w.Parent.Rule.Schedule) {
		Log(DEBUG, ctx, "RuleDone.Do", "location", loc.Name, "once", w.Parent.Rule.Id)
		_, err := loc.RemRule(ctx, w.Parent.Rule.Id)
		if err != nil {
			w.Disposition = &Condition{err.Error(), "unknown"}
			return
		}
	}
	w.Disposition = Complete
}

func (loc *Location) PrepareWork(ctx *Context, event Map) (*FindRules, error) {
	return &FindRules{Event: event}, nil
}

type Counter struct {
	sync.Mutex
	Count int
}

func (c *Counter) step() bool {
	c.Lock()
	ok := c.Count == -1
	if !ok {
		ok = 0 < c.Count
		c.Count--
	}
	c.Unlock()
	return ok
}

func NewCounter(steps int) *Counter {
	if steps == 0 {
		steps = -1
	}
	return &Counter{Count: steps}
}

func (loc *Location) WorkWalk(ctx *Context, w *FindRules, steps int) *Condition {
	c := NewCounter(steps)

	if w.Values == nil {
		w.Values = make([]interface{}, 0, 0)
	}

	if w.Disposition != Complete || c.step() {
		w.Do(ctx, loc)
	}
	if w.Disposition != Complete {
		return w.Disposition
	}

	for _, er := range w.Children {

		rule := er.Rule

		if er.Parent == nil {
			er.Parent = w
		}
		if er.Disposition != Complete || c.step() {
			er.Do(ctx, loc)
		}
		if er.Disposition != Complete {
			return er.Disposition
		}

		for _, erc := range er.Children {
			if erc.Parent == nil {
				erc.Parent = er
			}
			if erc.Disposition != Complete || c.step() {
				erc.Do(ctx, loc)
			}
			if erc.Disposition != Complete {
				return erc.Disposition
			}

			// Now we are finally down to the actions.

			if rule.Policies != nil && rule.Policies.SerialActions {
				for _, era := range erc.Children {
					if era.Parent == nil {
						era.Parent = erc
					}

					if era.Disposition != Complete || c.step() {
						era.Do(ctx, loc)
						if era.Disposition == Complete {
							w.Values = append(w.Values, era.Value)
						}
					}
					if era.Disposition != Complete {
						return era.Disposition
					}
				}
			} else { // Concurrent execution.
				wg := sync.WaitGroup{}
				vm := sync.Mutex{}
				wg.Add(len(erc.Children))
				for _, era := range erc.Children {
					go func(era *ExecRuleAction) {
						if era.Disposition != Complete || c.step() {
							era.Do(ctx, loc)
							if era.Disposition == Complete {
								vm.Lock()
								w.Values = append(w.Values, era.Value)
								vm.Unlock()
							}
						}
						wg.Done()
					}(era)
				}
				wg.Wait()
			}
		}

		// This rule's evaluation is complete.
		if er.DoneWork == nil {
			er.DoneWork = &RuleDone{Parent: er}
		}
		if er.DoneWork.Parent == nil {
			er.DoneWork.Parent = er
		}
		if er.DoneWork.Disposition != Complete || c.step() {
			er.DoneWork.Do(ctx, loc)
		}
		if er.DoneWork.Disposition != Complete {
			return er.DoneWork.Disposition
		}
	}

	return Complete
}

func (loc *Location) ProcessEvent(ctx *Context, event Map) (*FindRules, *Condition) {
	start, err := loc.PrepareWork(ctx, event)
	if err != nil {
		return start, &Condition{err.Error(), "unknown"}
	}
	cond := loc.WorkWalk(ctx, start, 0)
	if cond != nil && cond != Complete {
		return start, cond
	}
	return start, nil
}

func (loc *Location) RetryEventWork(ctx *Context, work *FindRules) *Condition {
	cond := loc.WorkWalk(ctx, work, 0)
	if cond != nil && cond != Complete {
		return cond
	}
	return nil
}
