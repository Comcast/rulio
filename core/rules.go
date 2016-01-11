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
	"encoding/json"
)

// RulePolicies are experimental switches that influence
// 'ProcessEvent' and the 'EventWorkStep' that it returns.
type RulePolicies struct {
	// RetryFromCondition will mark a rule's 'EvalRuleCondition'
	// as incomplete if any of that rule's actions fail.
	//
	// So if you submit that work again, the entire rule will be
	// re-evaluated starting the the rule's condition.
	//
	// NOT CURRENTLY IMPLEMENTED.
	RetryFromCondition bool `json:"retryFromCondition,omitempty"`

	// VerifyEnabled will demand that 'EventWorkStep' (and,
	// therefore, 'EventWorkWalk') will verify each rule is
	// enabled (and exists) before re-evaluating any part of the
	// rule.
	//
	// NOT CURRENTLY IMPLEMENTED.
	VerifyEnabled bool `json:"verifyEnabled,omitempty"`

	// SerialActions, if true, will cause rule processing
	// components to be executed sequentially.  Otherwise a rule's
	// actions will be executed concurrently.
	SerialActions bool `json:"serialActions,omitempty"`
}

// What a rule is.
//
// Note: Order is no longer supported (for several reasons).
//
// Note: 'Schedule' is not implemented (here).
type Rule struct {
	// The id for the rule.  The SHA1 has of the JSON is a good
	// 'id'.  That way the same rule added twice will not create
	// duplicate rules.
	Id string `json:"id,omitempty"`

	// Optional (but typical) pattern that will be matched against
	// incoming events.
	When *PatternQuery `json:"when,omitempty"`

	// Schedule is a crontab entry string or a duration starting with a "+".
	Schedule string `json:"schedule,omitempty"`

	// Optional query against facts (local or remote).
	Condition GenericQuery `json:"condition,omitempty"`

	// Actions: should have at least one.
	Actions []CleanAction `json:"actions,omitempty"`

	// Action just helps with unmarshalling.
	//
	// Will be stuck into Actions.
	Action *CleanAction `json:"action,omitempty"`

	Policies *RulePolicies `json:"policies,omitempty"`

	// Once specifies that the rule should be deleted after
	// exactly one evaluation.
	//
	// We didn't generalize to a count because we'd have to update
	// the rule state with each count decrement.
	Once bool `json:"once"`

	// Generic properties that are not currently used.
	Props map[string]interface{} `json:"props"`

	// Expires is an expiration time in UNIX seconds.
	//
	// 0 for no expiration.  This value is here to allow any
	// 'FindRules()' implementation drop expired rules.  Not all
	// implementations will need to consult this value.
	//
	// The type is float64 to deal with JSON serialization (and
	// since JSON only really has floats).
	Expires float64 `json:"expires"`

	// ToDo: Location?

	// See Rule.UnmarshalJSON() below.

	// But Zeus!  Whoever shouts “Zeus is victorious!” will gain
	// wisdom replete. Zeus it was who gave men their knowledge
	// and Zeus who made the rule, “pain is wisdom.”
	//
	//   --Aeschylus, in Agememnon
}

type CleanRule Rule

func (r *CleanRule) UnmarshalJSON(bs []byte) error {
	x := &Rule{}
	if err := json.Unmarshal(bs, x); err != nil {
		return err
	}

	r.Id = x.Id
	r.When = x.When
	r.Schedule = x.Schedule
	r.Condition = x.Condition
	r.Actions = x.Actions
	r.Policies = x.Policies
	r.Props = x.Props
	r.Expires = x.Expires

	if r.Action != nil {
		if r.Actions == nil {
			r.Actions = make([]CleanAction, 0, 0)
		}
		r.Actions = append(r.Actions, *r.Action)
		r.Action = nil
	}

	return nil
}

func (r *CleanRule) MarshalJSON(bs []byte) error {
	x := (*Rule)(r)
	x.Action = nil
	buf, err := json.Marshal(&x)
	if err != nil {
		return err
	}
	copy(bs, buf)
	return nil
}

// RuleToJSON generates a Rule from the given JSON representation.
func RuleFromJSON(ctx *Context, js []byte) (*Rule, error) {
	Log(DEBUG, ctx, "core.RuleFromJSON", "js", string(js))

	r := &Rule{}
	r.Condition.ctx = ctx
	if err := json.Unmarshal(js, r); err != nil {
		return nil, err
	}

	if r.When == nil && r.Schedule == "" {
		return nil, NewSyntaxError("need either a 'when' or a 'schedule'")
	}

	if r.When != nil && r.Schedule != "" {
		return nil, NewSyntaxError("can't have both a 'when' and a 'schedule'")
	}

	if r.Action != nil && r.Actions != nil {
		return nil, NewSyntaxError("specify either 'action' or 'actions'")
	}

	if r.Action != nil {
		r.Actions = []CleanAction{*r.Action}
		r.Action = nil
	}

	if r.Actions == nil || len(r.Actions) == 0 {
		return nil, NewSyntaxError("What good is a rule with no action?")
	}

	Log(DEBUG, ctx, "core.RuleFromMap", "rule", r)

	return r, nil
}

// RuleFromMap generates a Rule from a map.
func RuleFromMap(ctx *Context, m map[string]interface{}) (*Rule, error) {
	Log(DEBUG, ctx, "core.RuleFromMap", "m", m)
	bs, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return RuleFromJSON(ctx, bs)
}

// RuleToJSON generates a JSON representation of the given rule.
func RuleToJSON(ctx *Context, r Rule) ([]byte, error) {
	Log(DEBUG, ctx, "core.RuleToJSON", "rule", r)
	return json.Marshal(r)
}
