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
	"errors"
	"fmt"
	"math/rand"
	"sync"
)

var NoLocationProvider = errors.New("no location provider")

type Location struct {
	sync.RWMutex
	Name         string
	ReadOnly     bool
	Config       *Config
	control      *Control
	state        State
	stats        ServiceStats
	loading      bool
	lastUpdated  string
	updatedMutex sync.RWMutex

	// Provider is required when using parent locations.  Must be
	// set when the Location is created and then left unchanged.
	//
	// sys.System should be a good Provider.
	Provider LocationProvider
}

// Updated returns the in-memory timestamp of the last update.
func (loc *Location) Updated(ctx *Context) string {
	loc.updatedMutex.RLock()
	u := loc.lastUpdated
	loc.updatedMutex.RUnlock()
	Log(INFO, ctx, "Location.Updated", "updated", u)
	return u
}

// GenerateUpdateTimestamp returns a string consisting of the current
// timestamp and 'AnIpAddress'.
//
// Used by 'Update()'.
func GenerateUpdateTimestamp(ctx *Context) string {
	return NowString() + "_" + AnIpAddress
}

// Update sets the in-memory timestamp of the last update.
//
// If 'updating' is empty, then a value is generated using
// 'GenerateUpdateTimestamp()'.  If 'updating' is "clear", then unset
// that state.  Otherwise, the value of 'update' will be stored as the
// time the location was last updated.
//
// Returns the previous value and the new value.
func (loc *Location) Update(ctx *Context, updating string) (string, string) {
	clear := updating == "clear"
	if updating == "" {
		updating = GenerateUpdateTimestamp(ctx)
	}
	Log(INFO, ctx, "Location.Update", "updating", updating)
	loc.updatedMutex.Lock()
	lastUpdated := loc.lastUpdated
	if clear {
		loc.lastUpdated = ""
	} else {
		loc.lastUpdated = updating
	}
	loc.updatedMutex.Unlock()
	return lastUpdated, updating
}

func (loc *Location) Control() *Control {
	// ToDo: Consider sync.atomic.LoadPointer() or sync.atomic.Value.
	loc.RLock()
	p := loc.control
	if p == nil {
		// Watch out: allocation that perhaps we don't want.
		p = SystemParameters.DefaultControl
		if p == nil {
			p = DefaultControl()
		}
		loc.control = p
	}
	loc.RUnlock()
	return p
}

func (loc *Location) SetControl(c *Control) *Control {
	// ToDo: Consider sync.atomic.StorePointer() or sync.atomic.Value.
	loc.Lock()
	loc.control = c
	loc.Unlock()
	return c
}

func (loc *Location) SetReadOnly(ctx *Context, readOnly bool) {
	Log(INFO, ctx, "Location.SetReadOnly", "location", loc.Name, "readOnly", readOnly)
	loc.Lock()
	loc.ReadOnly = readOnly
	loc.Unlock()
}

func (loc *Location) IsReadOnly(ctx *Context) bool {
	loc.RLock()
	readOnly := loc.ReadOnly
	loc.RUnlock()
	return readOnly
}

func (loc *Location) CheckWrite(ctx *Context) error {
	Log(DEBUG, ctx, "Location.CheckWrite", "location", loc.Name)
	if loc.IsReadOnly(ctx) {
		return fmt.Errorf("Read only")
	}
	key, _, _ := GetPropString(ctx, loc.state, "writeKey", "")
	Log(DEBUG, ctx, "Location.CheckWrite", "location", loc.Name, "key", key)
	if key == "" || ctx.WriteKey == key {
		return nil
	}
	return fmt.Errorf("Write operation not allowed by key")
}

func (loc *Location) CheckRead(ctx *Context) error {
	Log(DEBUG, ctx, "Location.CheckRead", "location", loc.Name)
	key, _, _ := GetPropString(ctx, loc.state, "readKey", "")
	Log(DEBUG, ctx, "Location.CheckRead", "location", loc.Name, "key", key)
	if key == "" || ctx.ReadKey == key {
		return nil
	}
	return fmt.Errorf("Read operation not allowed by key")
}

func NewLocation(ctx *Context, name string, state State, ctrl *Control) (*Location, error) {
	Log(INFO, ctx, "core.NewLocation", "location", name)

	// Kinda trying to retrofit this API to allow -- but not
	// require -- the caller to provide the State.

	if state == nil {
		// We'll try to make something.
		store, _ := NewMemStorage(ctx)
		var err error
		state, err = NewLinearState(ctx, name, store)
		if err != nil {
			Log(ERROR, ctx, "core.NewLocation", "error", err, "location", name)
			return nil, err
		}
	}

	// ToDo: CacheExpires default duration.
	// loc := Location{sync.RWMutex{}, name, false, nil, ctrl, state, ServiceStats{}, false}
	loc := Location{sync.RWMutex{}, name, false, nil, nil, state, ServiceStats{}, false, "", sync.RWMutex{}, nil}

	return &loc, loc.init(ctx)
}

func (loc *Location) init(ctx *Context) error {
	Log(INFO, ctx, "Location.Init", "location", loc.Name)

	ctx.SetLoc(loc)
	loc.loading = true
	if err := loc.state.Load(ctx); err != nil {
		Log(ERROR, ctx, "Location.init", "error", err, "when", "State.Load", "location", loc.Name)
	}
	loc.loading = false

	return nil
}

func (loc *Location) StateSize(ctx *Context) (int, error) {
	if err := loc.CheckRead(ctx); err != nil {
		return 0, err
	}
	return loc.state.Count(ctx), nil
}

func (loc *Location) AtCapacity(ctx *Context) bool {
	// Also see TermIndexMetrics.
	// ToDo: Consider termindex and patindex sizes.
	at := loc.Control().MaxFacts <= loc.state.Count(ctx)
	Log(DEBUG, ctx, "Location.AtCapacity", "location", loc.Name, "full", at)
	return at
}

func (loc *Location) Enabled(ctx *Context) bool {
	enabled, _, _ := GetPropString(ctx, loc.state, "enabled", "")
	yes := enabled == "" || enabled == "yes" || enabled == "true"
	if !yes {
		Log(WARN, ctx, "Location.Enabled", "location", loc.Name, "enabled", yes)
	}
	return yes
}

// AlwaysHaveRule is a hack to disable Location.Have(), which doesn't
// work when we're dealing with a parent rule.
var AlwaysHaveRule = true

func (loc *Location) Have(ctx *Context, id string, isRule bool) (bool, error) {
	if AlwaysHaveRule {
		return true, nil
	}
	x, err := loc.state.Get(ctx, id)
	if _, missing := err.(*NotFoundError); missing {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if isRule {
		if _, rule := x["rule"]; rule {
			return true, nil
		}
		return false, nil
	}
	return true, nil
}

func (loc *Location) RuleEnabled(ctx *Context, id string) (bool, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.RuleEnabled", "location", loc.Name, "ruleId", id)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.RuleEnabled", "location", loc.Name, "uerr", "disabled", "ruleId", id)
		return false, fmt.Errorf("Location is disabled.")
	}

	Inc(&loc.stats.TotalCalls, 1)
	var err error
	enabled := true
	var have bool
	have, err = loc.Have(ctx, id, true)
	if !have {
		enabled = false
		Log(INFO, ctx, "Location.RuleEnabled", "location", loc.Name, "found", false)
		err = NewNotFoundError(id)
	} else {
		var got interface{}
		got, _, err = GetProp(ctx, loc.state, id, "disabled", false)
		disabled, ok := got.(bool)
		if ok {
			enabled = !disabled
		} else {
			Log(WARN, ctx, "Location.RuleEnabled", "location", loc.Name, "disabled", got)
		}
	}
	loc.stats.IncErrors(err)
	return enabled, err
}

func (loc *Location) SetProp(ctx *Context, id string, prop string, val interface{}) error {
	_, err := SetProp(ctx, loc.state, id, prop, val)
	return err
}

func (loc *Location) RemProp(ctx *Context, id string, prop string) error {
	_, err := RemProp(ctx, loc.state, id, prop)
	return err
}

func (loc *Location) EnableRule(ctx *Context, id string, enable bool) error {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.EnableRule", "location", loc.Name, "ruleId", id, "enable", enable)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.AddRule", "location", loc.Name, "uerr", "disabled", "ruleId", id)
		return fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return err
	}

	Metric(ctx, "RuleEnabled", "location", loc.Name, "ruleId", id, "enabled", enable)

	timer := NewTimer(ctx, "EnableRule")
	Inc(&loc.stats.TotalCalls, 1)
	var err error
	var have bool
	have, err = loc.Have(ctx, id, true)
	if !have {
		err = NewNotFoundError(id)
	} else {
		if enable {
			_, err = RemProp(ctx, loc.state, id, "disabled")
		} else {
			_, err = SetProp(ctx, loc.state, id, "disabled", true)
		}
	}
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return err
}

func (loc *Location) AddRule(ctx *Context, id string, rule Map) (string, error) {
	// Any fool can make a rule
	// And any fool will mind it.
	//
	//  --Henry David Thoreau

	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.AddRule", "location", loc.Name, "ruleId", id, "rule", rule)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.AddRule", "location", loc.Name, "uerr", "disabled", "ruleId", id)
		return "", fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return "", err
	}

	timer := NewTimer(ctx, "AddRule")
	Inc(&loc.stats.TotalCalls, 1)
	var err error
	if loc.AtCapacity(ctx) {
		max := loc.Control().MaxFacts
		err = fmt.Errorf("Location state capacity limit reached (%d)", max)
		Log(WARN, ctx, "Location.AddRule", "location", loc.Name, "uerr", err, "rule", rule, "ruleId", id, "when", "capacity")
		return "", err
	}

	// Validate the rule
	if _, err = RuleFromMap(ctx, rule); err != nil {
		Log(UERR, ctx, "Location.AddRule", "location", loc.Name, "uerr", err, "rule", rule, "ruleId", id)
		return "", err
	}

	Inc(&loc.stats.AddRules, 1)

	expiring, expires, err := setExpires(ctx, rule)
	if err != nil {
		Log(UERR, ctx, "Location.AddRule", "location", loc.Name, "uerr", err, "rule", rule, "ruleId", id)
		return "", err
	}

	wrapper := make(map[string]interface{})
	wrapper["rule"] = map[string]interface{}(rule)

	if expiring {
		wrapper["expires"] = expires
	}

	// Add deleteWith to the top level so it can be picked up when
	// deleting dependencies (as with any fact).
	if deleteWith, given := rule[KW_DeleteWith]; given {
		wrapper[KW_DeleteWith] = deleteWith
	}

	Log(DEBUG, ctx, "Location.AddRule", "location", loc.Name, "wrapper", wrapper, "ruleId", id)
	id, err = loc.state.Add(ctx, id, wrapper)
	if err != nil {
		Log(ERROR, ctx, "Location.AddRule", "location", loc.Name, "error", err, "fact", wrapper, "ruleId", id)
	}

	if err == nil {
		Metric(ctx, "RuleCreated", "location", loc.Name, "ruleId", id)
	}

	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return id, err
}

func (loc *Location) RemRule(ctx *Context, id string) (string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.RemRule", "location", loc.Name, "ruleId", id)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.RemRule", "location", loc.Name, "uerr", "disabled", "ruleId", id)
		return "", fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return "", err
	}

	timer := NewTimer(ctx, "RemRule")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.RemRules, 1)
	_, err := loc.state.Rem(ctx, id)
	if err == nil {
		var have bool
		_, have, err = GetProp(ctx, loc.state, id, "disabled", false)
		if have {
			_, err = RemProp(ctx, loc.state, id, "disabled")
		}
	}
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return id, err
}

func ExtractRule(ctx *Context, fact Map, required bool) (Map, error) {
	rule, have := fact["rule"]
	if have {
		switch vv := rule.(type) {
		case map[string]interface{}:
			expires, have := fact["expires"]
			Log(DEBUG, ctx, "ExtractRule", "expires", expires)
			if have {
				// ToDo: Probably shouldn't modify given fact this way.
				vv["expires"] = expires
			}
			return vv, nil
		default:
			if required {
				err := fmt.Errorf("Internal error: Rule body %v is a %T not a rule", rule, rule)
				Log(ERROR, ctx, "ExtractRule", "error", err, "fact", fact)
				return nil, err
			}
		}
	}
	if required {
		err := fmt.Errorf("Internal error: Rule body missing from %v", fact)
		Log(ERROR, ctx, "ExtractRule.FindRules", "error", err, "fact", fact)
		return nil, err
	}
	return nil, nil
}

func (loc *Location) GetRule(ctx *Context, id string) (Map, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.GetRule", "location", loc.Name, "ruleId", id)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.GetRule", "location", loc.Name, "uerr", "disabled", "ruleId", id)
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}
	timer := NewTimer(ctx, "GetRule")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.GetRules, 1)

	fact, err := loc.state.Get(ctx, id)
	var rule Map
	if err == nil {
		rule, err = ExtractRule(ctx, fact, true)
	}

	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return rule, err
}

func (loc *Location) AddFact(ctx *Context, id string, fact Map) (string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.AddFact", "location", loc.Name, "id", id)
	if err := loc.CheckWrite(ctx); err != nil {
		return "", err
	}
	if loc.AtCapacity(ctx) {
		max := loc.Control().MaxFacts
		err := fmt.Errorf("Location state capacity limit reached (%d)", max)
		Log(WARN, ctx, "Location.AddFact", "location", loc.Name, "uerr", err, "js", fact, "id", id, "when", "capacity")
		return "", err
	}
	id, err := loc.addFact(ctx, id, fact)
	if err == nil {
		Log(DEBUG, ctx, "Location.AddFact", "location", loc.Name, "factid", id, "bytes", len(fact))
	} else {
		Log(ERROR, ctx, "Location.AddFact", "location", loc.Name, "factid", id, "error", err)
	}
	return id, err
}

func (loc *Location) addFact(ctx *Context, id string, fact Map) (string, error) {
	ctx.SetLoc(loc)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.AddFact", "location", loc.Name, "uerr", "disabled", "id", id, "factjs", fact)
		return "", fmt.Errorf("Location is disabled.")
	}
	timer := NewTimer(ctx, "AddFact")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.AddFacts, 1)
	var err error
	id, err = loc.state.Add(ctx, id, fact)
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return id, err
}

func (loc *Location) RemFact(ctx *Context, id string) (string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.RemFact", "location", loc.Name, "id", id)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.RemFact", "location", loc.Name, "uerr", "disabled", "id", id)
		return "", fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return "", err
	}
	timer := NewTimer(ctx, "RemFact")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.RemFacts, 1)
	_, err := loc.state.Rem(ctx, id)
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return id, err
}

func (loc *Location) GetFact(ctx *Context, id string) (Map, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.GetFact", "location", loc.Name, "id", id)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.GetFact", "location", loc.Name, "uerr", "disabled", "id", id)
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}
	timer := NewTimer(ctx, "GetFact")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.GetFacts, 1)
	fact, err := loc.state.Get(ctx, id)
	if err != nil {
		Log(ERROR, ctx, "Location.GetFact", "error", err, "location", loc.Name, "fact", fact, "id", id)
	}
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return fact, err
}

// DoAncestors calls the given function on this location and all of its ancestors in depth-first order.
func (loc *Location) DoAncestors(ctx *Context, fn func(*Location) error) error {

	parents, err := loc.getParents(ctx)
	if err != nil {
		return err
	}

	if 0 < len(parents) {
		if loc.Provider == nil {
			// If we don't have a LocationProvider, we have no hope of getting any parent locations.
			return NoLocationProvider
		}

		for _, parent := range parents {
			p, err := loc.Provider.GetLocation(ctx, parent)
			if err != nil {
				return err
			}
			if err = p.DoAncestors(ctx, fn); err != nil {
				return err
			}
		}
	}

	return fn(loc)
}

func (loc *Location) searchFactsAncestors(ctx *Context, pattern Map) (*SearchResults, error) {
	srs := &SearchResults{
		Found: make([]SearchResult, 0, 64),
	}

	// Local rules shadow those from parents.
	err := loc.DoAncestors(ctx, func(parent *Location) error {
		more, err := parent.searchFacts(ctx, pattern)
		if err != nil {
			return err
		}

		srs.Merge(more)

		return nil
	})

	return srs, err
}

func (loc *Location) searchFacts(ctx *Context, pattern Map) (*SearchResults, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.searchFacts", "location", loc.Name, "pattern", pattern)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.searchFacts", "location", loc.Name, "uerr", "disabled", "pattern", pattern)
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}
	timer := NewTimer(ctx, "searchFacts")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.SearchFacts, 1)
	sr, err := loc.state.Search(ctx, pattern)
	loc.stats.IncErrors(err)

	Inc(&loc.stats.TotalTime, timer.Stop())
	return sr, err
}

func (loc *Location) SearchFacts(ctx *Context, pattern Map, includeInherited bool) (*SearchResults, error) {
	if includeInherited {
		return loc.searchFactsAncestors(ctx, pattern)
	} else {
		return loc.searchFacts(ctx, pattern)
	}
}

func duplicateId(msg string) error {
	// ToDo: Probably make a real struct to implement this error.
	return fmt.Errorf("duplicate id %s", msg)
}

func (loc *Location) searchRulesAncestors(ctx *Context, event Map) (map[string]Map, error) {
	acc := make(map[string]Map)

	// Local rules shadow those from parents.
	err := loc.DoAncestors(ctx, func(parent *Location) error {
		more, err := parent.searchRules(ctx, event)
		if err != nil {
			return err
		}

		for id, x := range more {
			if _, have := acc[id]; have {
				return duplicateId(loc.Name + " and " + parent.Name + " have '" + id + "'")
			}
			acc[id] = x
		}

		return nil
	})

	return acc, err
}

func (loc *Location) searchRules(ctx *Context, event Map) (map[string]Map, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.SearchRules", "location", loc.Name, "event", event)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.SearchRules", "location", loc.Name, "uerr", "disabled", "em", event)
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}
	timer := NewTimer(ctx, "SearchRules")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.SearchRules, 1)

	var err error

	acc, err := loc.state.FindRules(ctx, event)
	if err != nil {
		Log(ERROR, ctx, "Location.SearchRules", "error", err, "location", loc.Name)
	}

	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return acc, err
}

func (loc *Location) SearchRules(ctx *Context, event Map, includeInherited bool) (map[string]Map, error) {
	// Chorus: Who then is ruler of necessity?
	// Prometheus: The triple Fates and unforgetting Furies.
	//
	//   --Aeschylus, in Prometheus Chained.

	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.SearchRules", "location", loc.Name, "event", event)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.SearchRules", "location", loc.Name, "uerr", "disabled", "em", event)
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}
	timer := NewTimer(ctx, "SearchRules")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.SearchRules, 1)

	var acc map[string]Map
	var err error
	if includeInherited {
		acc, err = loc.searchRulesAncestors(ctx, event)
	} else {
		acc, err = loc.searchRules(ctx, event)
	}

	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return acc, err
}

func (loc *Location) ListRules(ctx *Context, includeInherited bool) ([]string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.ListRules", "location", loc.Name)

	// Now it's Search
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.ListRules", "location", loc.Name, "uerr", "disabled")
		return nil, fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckRead(ctx); err != nil {
		return nil, err
	}

	timer := NewTimer(ctx, "ListRules")
	Inc(&loc.stats.TotalCalls, 1)
	Inc(&loc.stats.ListRules, 1)

	sr, err := loc.SearchFacts(ctx, Map{"rule": "?rule"}, includeInherited)

	acc := make([]string, 0, len(sr.Found))
	if err == nil {
		for _, srs := range sr.Found {
			// ToDo: Be more careful
			rule, _ := srs.Bindingss[0]["?rule"]
			switch rule.(type) {
			case string, map[string]interface{}:
				acc = append(acc, srs.Id)
			default:
				err = fmt.Errorf("Wanted a string but got %v (%T)", rule, rule)
				break
			}
		}
	}

	if err != nil {
		Log(ERROR, ctx, "Location.ListRules", "error", err, "location", loc.Name)
	}

	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return acc, nil
}

// getParents is current just a wrapper around 'GetProp' to read a
// location's parents (if any).
func (loc *Location) getParents(ctx *Context) ([]string, error) {
	x, have, err := GetProp(ctx, loc.state, "", "parents", make([]string, 0, 0))
	if err != nil {
		return nil, err
	}
	if !have {
		return nil, nil
	}
	parents, ok := x.([]string)
	if !ok {
		xs, ok := x.([]interface{})
		if !ok {
			return nil, fmt.Errorf("didn't expect parents %#v", x)
		}
		parents = make([]string, len(xs))
		for i, x := range xs {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("didn't expect parent %#v", x)
			}
			parents[i] = s
		}
	}

	return parents, nil
}

// GetParents does what you'd think.
//
// Maybe shouldn't be a top-level location API.  Instead expose
// 'GetProp' and document special properties?
func (loc *Location) GetParents(ctx *Context) ([]string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.GetParents", "location", loc.Name)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.GetParents", "location", loc.Name)
		return nil, fmt.Errorf("Location is disabled.")
	}

	Metric(ctx, "GetParents", "location", loc.Name)

	timer := NewTimer(ctx, "GetParents")
	Inc(&loc.stats.TotalCalls, 1)
	parents, err := loc.getParents(ctx)
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return parents, err
}

// setParents is currently just a wrapper for 'SetProp' to set a location's parents.
//
// To remove a location's parents, just pass a zero array.
func (loc *Location) setParents(ctx *Context, parents []string) (string, error) {
	// We might want to search for this property as a fact.  If
	// so, our pattern matching code (as it's currently written)
	// will fail to match and []interface{} of strings and an
	// otherwise-matching []string.
	//
	// So we do
	ps := make([]interface{}, len(parents))
	for i, p := range parents {
		ps[i] = p
	}
	return SetProp(ctx, loc.state, "", "parents", ps)
}

// SetParents sets a location's parents.
//
// To remove a location's parents, just pass a zero array.
//
// See comments re 'GetParents'.
func (loc *Location) SetParents(ctx *Context, parents []string) (string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.SetParents", "location", loc.Name)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.SetParents", "location", loc.Name)
		return "", fmt.Errorf("Location is disabled.")
	}

	Metric(ctx, "SetParents", "location", loc.Name)

	timer := NewTimer(ctx, "SetParents")
	Inc(&loc.stats.TotalCalls, 1)
	id, err := loc.setParents(ctx, parents)
	loc.stats.IncErrors(err)
	Inc(&loc.stats.TotalTime, timer.Stop())
	return id, err
}

func (loc *Location) Clear(ctx *Context) error {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.Clear", "location", loc.Name)

	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.Clear", "location", loc.Name, "uerr", "disabled")
		return fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return err
	}
	timer := NewTimer(ctx, "Clear")
	Inc(&loc.stats.TotalCalls, 1)
	err := loc.state.Clear(ctx)
	if err = loc.stats.IncErrors(err); err != nil {
		Log(ERROR, ctx, "Location.Clear", "location", loc.Name, "error", err)
	}
	Inc(&loc.stats.TotalTime, timer.Stop())
	return err
}

func (loc *Location) RunJavascript(ctx *Context, code string, libraries []string, bs *Bindings, props map[string]interface{}) (interface{}, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.RunJavascript", "location", loc.Name)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.RunJavascript", "location", loc.Name, "uerr", "disabled")
		return nil, fmt.Errorf("Location is disabled.")
	}

	timer := NewTimer(ctx, "LocationRunJavascript")
	Inc(&loc.stats.TotalCalls, 1)

	script, err := CompileJavascript(ctx, loc, libraries, code)
	if nil != err {
		Log(ERROR, ctx, "Location.RunJavascript", "location", loc.Name, "error", err)
		return nil, err
	}

	x, err := RunJavascript(ctx, bs, props, script)
	if err != nil {
		Log(ERROR, ctx, "Location.RunJavascript", "location", loc.Name, "error", err)
		return nil, err
	}
	Inc(&loc.stats.TotalTime, timer.Stop())
	return x, err
}

func (loc *Location) Query(ctx *Context, query string) (*QueryResult, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.Query", "location", loc.Name)
	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.Query", "location", loc.Name, "uerr", "disabled")
		return nil, fmt.Errorf("Location is disabled.")
	}

	timer := NewTimer(ctx, "Query")
	Inc(&loc.stats.TotalCalls, 1)

	var qr *QueryResult
	var m map[string]interface{}
	err := json.Unmarshal([]byte(query), &m)
	if err == nil {
		var q Query
		q, err = ParseQuery(ctx, m)
		if err == nil {
			iqr := InitialQueryResult(ctx)
			qc := QueryContext{Locations: []string{loc.Name}}
			qr, err = ExecQuery(ctx, q, loc, qc, iqr)
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.Query", "location", loc.Name, "query", query, "error", err)
	}
	Inc(&loc.stats.TotalTime, timer.Stop())
	return qr, err
}

func (loc *Location) Delete(ctx *Context) error {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.Delete", "location", loc.Name)

	if !loc.Enabled(ctx) {
		Log(WARN, ctx, "Location.Delete", "location", loc.Name, "uerr", "disabled")
		return fmt.Errorf("Location is disabled.")
	}
	if err := loc.CheckWrite(ctx); err != nil {
		return err
	}
	timer := NewTimer(ctx, "Delete")
	Inc(&loc.stats.TotalCalls, 1)
	err := loc.state.Delete(ctx)
	if err = loc.stats.IncErrors(err); err != nil {
		Log(ERROR, ctx, "Location.Delete", "location", loc.Name, "error", err)
	}
	Inc(&loc.stats.TotalTime, timer.Stop())
	return err
}

func (loc *Location) ResolveService(ctx *Context, name string) (string, error) {
	ctx.SetLoc(loc)
	Log(INFO, ctx, "Location.ResolveService", "service", name)

	if loc.Control().Services == nil {
		Log(WARN, ctx, "Location.ResolveService", "service", "nil")
		return name, nil
	}

	urls, given := loc.Control().Services[name]
	if given {
		url := urls[rand.Intn(len(urls))]
		Log(DEBUG, ctx, "System.resolveService", "service", name, "urls", urls, "url", url)
		return url, nil
	}
	return name, nil
}

func (loc *Location) GetPropString(ctx *Context, prop string, def string) (string, bool, error) {
	// Exposed so System can quickly see who owns this location.
	return GetPropString(ctx, loc.state, prop, def)
}

func (loc *Location) GetProp(ctx *Context, prop string, def interface{}) (interface{}, bool, error) {
	return GetProp(ctx, loc.state, "", prop, def)
}

func (loc *Location) Stats() *ServiceStats {
	return loc.stats.Clone()
}

func (loc *Location) ClearStats() {
	Log(INFO, nil, "Location.ClearStats", "location", loc.Name)
	loc.stats.Reset()
}

type LocationProvider interface {
	GetLocation(ctx *Context, name string) (*Location, error)
}

type SimpleLocationProvider struct {
	Registry map[string]*Location
}

func NewSimpleLocationProvider(locs map[string]*Location) *SimpleLocationProvider {
	return &SimpleLocationProvider{locs}
}

func (p *SimpleLocationProvider) GetLocation(ctx *Context, name string) (*Location, error) {
	loc, have := p.Registry[name]
	if !have {
		return nil, NewNotFoundError(name)
	}
	return loc, nil
}
