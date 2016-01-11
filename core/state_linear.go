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
	"fmt"
	"sync"
	"time"
)

type RawFact struct {
	M  map[string]interface{}
	JS []byte
}

type LinearState struct {
	sync.RWMutex

	// Name should probably the name of the location for this state.
	Name string

	// Loc points to the state's location in order to call back to
	// the location for certain operations.
	loc *Location

	Facts map[string]RawFact

	store Storage

	addHook AddHookFn

	remHook RemHookFn
}

func (s *LinearState) withPrivilege(ctx *Context) {
	ctx.grantPrivilege("hook")
}

func (s *LinearState) withoutPrivilege(ctx *Context) {
	ctx.revokePrivilege()
}

func (s *LinearState) slock(ctx *Context, read bool) {
	Log(DEBUG, ctx, "LinearState.slock", "read", read, "priv", ctx.isPrivileged("hook"))
	if ctx != nil && ctx.isPrivileged("hook") {
		return
	}
	if read {
		s.RLock()
	} else {
		s.Lock()
	}
}

func (s *LinearState) sunlock(ctx *Context, read bool) {
	Log(DEBUG, ctx, "LinearState.sunlock", "read", read, "priv", ctx.isPrivileged("hook"))
	if ctx != nil && ctx.isPrivileged("hook") {
		return
	}
	if read {
		s.RUnlock()
	} else {
		s.Unlock()
	}
}

func NewLinearState(ctx *Context, name string, store Storage) (*LinearState, error) {
	s := &LinearState{}
	s.Name = name
	s.store = store
	s.Facts = make(map[string]RawFact)
	return s, nil
}

func (s *LinearState) AddHook(hook AddHookFn) {
	s.addHook = hook
}

func (s *LinearState) RemHook(hook RemHookFn) {
	s.remHook = hook
}

// Count returns an approximate count of the number of facts.
//
// Only an approximation because we will count expired facts that have
// not been removed.
func (s *LinearState) Count(ctx *Context) int {
	s.slock(ctx, true)
	n := len(s.Facts)
	s.sunlock(ctx, true)
	return n
}

func (s *LinearState) IsLoaded(ctx *Context) bool {
	s.slock(ctx, false)
	loaded := s.Facts != nil
	s.sunlock(ctx, false)
	return loaded
}

func (s *LinearState) Load(ctx *Context) error {
	name := "unknown"
	if ctx.GetLoc() != nil {
		name = ctx.GetLoc().Name
	}
	Log(INFO, ctx, "LinearState.Load", "location", name)

	// Could TTL here.
	timer := NewTimer(ctx, "LinearState.Load")
	// ToDo: Combine defered code?
	defer timer.Stop()

	s.slock(ctx, false)
	defer s.sunlock(ctx, false)

	pairs, err := s.store.Load(ctx, s.Name)
	if err != nil {
		return err
	}
	s.Facts = make(map[string]RawFact, len(pairs))
	for _, pair := range pairs {
		id := string(pair.K)
		js := pair.V
		var m Map
		err = json.Unmarshal(js, &m)
		if err != nil {
			Log(DEBUG, ctx, "LinearState.Load", "error", err, "js", string(js))
			return err
		}
		s.Facts[id] = RawFact{m, js}
	}

	Log(DEBUG, ctx, "LinearState.Load", "location", s.Name, "facts", len(s.Facts))
	return nil
}

func (s *LinearState) Add(ctx *Context, id string, x Map) (string, error) {
	Log(DEBUG, ctx, "LinearState.Add", "state", s.Name, "x", x, "id", id)

	timer := NewTimer(ctx, "LinearState.Add")
	defer timer.Stop()

	id, m, err := PrepareFact(ctx, id, x)
	if err != nil {
		return id, err
	}

	bs, err := json.Marshal(&x)
	if err != nil {
		return id, err
	}

	pair := &Pair{[]byte(id), bs}
	if err = s.store.Add(ctx, s.Name, pair); err != nil {
		return id, err
	}

	if s.addHook != nil {
		if err := s.addHook(ctx, s, id, m, ctx.GetLoc().loading); err != nil {
			Log(ERROR, ctx, "LinearState.Add", "state", s.Name, "error", err, "when", "addHook", "id", id)
			return "", err
		}
	}

	// Maybe protect the store (above), too.
	s.slock(ctx, false)
	if _, isRule := m["rule"]; isRule {
		if _, have := s.Facts[id]; have {
			// Hope we're really replacing a rule.
			Metric(ctx, "RuleUpdated", "location", s.Name, "ruleId", id)
		}
	}
	s.Facts[id] = RawFact{m, bs}
	s.sunlock(ctx, false)

	return id, nil
}

func (s *LinearState) Rem(ctx *Context, id string) (bool, error) {
	Log(DEBUG, ctx, "LinearState.Rem", "id", id)
	timer := NewTimer(ctx, "LinearState.Rem")
	defer timer.Stop()

	if s.remHook != nil {
		if err := s.remHook(ctx, s, id); err != nil {
			Log(ERROR, ctx, "LinearState.Rem", "state", s.Name, "error", err,
				"id", id, "when", "remHook")
			// ToDo: Consider queueing, falling through, ...
			return false, err
		}
	}

	return s.rem(ctx, id, true)
}

func (s *LinearState) rem(ctx *Context, id string, lock bool) (bool, error) {
	Log(DEBUG, ctx, "LinearState.rem", "id", id)
	_, err := s.store.Remove(ctx, s.Name, []byte(id))
	// ToDo: Consider what's returned.
	if err != nil {
		Log(ERROR, ctx, "LinearState.rem", "id", id, "error", err)
		return false, err
	}
	// Maybe protect the store (above), too.
	if lock {
		s.slock(ctx, false)
		defer s.sunlock(ctx, false)
	}
	_, had := s.Facts[id]
	if had {
		Log(DEBUG, ctx, "LinearState.Rem", "found", id)
	} else {
		Log(DEBUG, ctx, "LinearState.Rem", "missing", id)
	}
	delete(s.Facts, id)
	// At least for now, we delete dependencies AFTER deleting the
	// requested fact.  Otherwise we risk loops that are expensive
	// to detect.
	if err := s.deleteDependencies(ctx, id); err != nil {
		Log(WARN, ctx, "LinearState.Rem", "error", err, "when", "deleteDependencies", "id", id)
		return had, err
	}
	return had, nil
}

func (s *LinearState) deleteDependencies(ctx *Context, id string) error {
	Log(DEBUG, ctx, "LinearState.deleteDependencies", "id", id)
	pattern := Map{
		KW_DeleteWith: []string{id},
	}
	srs, err := s.search(ctx, pattern, false)
	if nil != err {
		return err
	}

	for _, sr := range srs.Found {
		if id == sr.Id {
			Log(WARN, ctx, "LinearState.deleteDependencies", "loop", id)
			continue
		}
		if _, err := s.rem(ctx, sr.Id, false); nil != err {
			return err
		}
	}

	return nil
}

func (s *LinearState) Search(ctx *Context, pattern Map) (*SearchResults, error) {
	return s.search(ctx, pattern, true)
}

func (s *LinearState) search(ctx *Context, pattern Map, lock bool) (*SearchResults, error) {
	Log(DEBUG, ctx, "LinearState.Search", "pattern", pattern)
	timer := NewTimer(ctx, "LinearState.search")
	defer timer.Stop()

	now := time.Now().UTC().Unix()
	// ToDo: Mutex

	srs := SearchResults{}
	srs.Found = make([]SearchResult, 0, 0)
	if lock {
		s.slock(ctx, true)
		defer s.sunlock(ctx, true)
	}
	for id, rf := range s.Facts {
		srs.Checked++
		expired, err := s.expire(ctx, id, rf.M, now)
		if err != nil {
			return nil, err
		}
		if expired {
			srs.Expired++
			continue
		}
		bss, err := Matches(ctx, pattern, rf.M)
		if err != nil {
			Log(WARN, ctx, "LinearState.search", "name", s.Name, "error", err)
			return nil, err
		}
		js := rf.JS
		// Do the injection after the match -- as we do with IndexedState.
		if maybeInjectId(ctx, id, rf.M, false) {
			// Have to rewrite the JSON.
			js, err = json.Marshal(&rf.M)
			if err != nil {
				Log(WARN, ctx, "LinearState.search", "name", s.Name, "error", err)
				return nil, err
			}
		}

		if 0 < len(bss) {
			sr := SearchResult{string(js), id, bss}
			srs.Found = append(srs.Found, sr)
		}
	}

	Log(DEBUG, ctx, "LinearState.search", "srs", srs)

	if ctx != nil && ctx.GetLoc() != nil {
		c := ctx.GetLoc().Control()
		if c != nil {
			if 0 < c.BindingsWarningLimit && c.BindingsWarningLimit < len(srs.Found) {
				Log(WARN, ctx, "LinearState.search", "name", s.Name, "bindingsWarning", len(srs.Found))
			}
		}
	}

	return &srs, nil
}

func (s *LinearState) FindRules(ctx *Context, event Map) (map[string]Map, error) {
	Log(DEBUG, ctx, "LinearState.FindRules", "name", s.Name, "event", event)
	timer := NewTimer(ctx, "LinearState.FindRules")
	defer timer.Stop()

	// We could call Search(), but we'll try to be a bit
	// more efficient here.
	acc := make(map[string]Map)
	s.slock(ctx, true)
	defer s.sunlock(ctx, true)
	now := time.Now().UTC().Unix()
	for id, rf := range s.Facts {
		rule, given := rf.M["rule"]
		if !given {
			continue
		}
		expired, err := s.expire(ctx, id, rf.M, now)
		if err != nil {
			Log(ERROR, ctx, "LinearState.FindRules", "error", err, "when", "expiring")
			return nil, err
		}
		if expired {
			Log(DEBUG, ctx, "LinearState.FindRules", "expired", expired, "ruleId", id, "rule", rule)
			continue
		}

		switch r := rule.(type) {
		case map[string]interface{}:
			when, given := r["when"]
			if !given {
				continue
			}
			switch when.(type) {
			case Map, map[string]interface{}:
				p, ok := when.(map[string]interface{})
				if !ok {
					p = map[string]interface{}(when.(Map))
				}
				pattern, given := p["pattern"]
				if !given {
					Log(WARN, ctx, "LinearState.FindRules", "name", s.Name, "pattern", "implicit")
					pattern = p
				}

				// ToDo: We are duplicating some this
				// work elsewhere.  Don't.  Would be
				// nice to be able to annotate our
				// results as already match-processed.
				bss, err := Matches(ctx, pattern, event)
				if err != nil {
					return nil, err
				}
				if 0 < len(bss) {
					acc[id] = r
				}
			}
		default:
			panic(fmt.Errorf("rule %#v bad type", rule))
		}
	}

	return acc, nil
}

func (s *LinearState) Clear(ctx *Context) error {
	Log(INFO, ctx, "LinearState.Clear", "name", s.Name)
	_, err := s.store.Clear(ctx, s.Name)
	// Maybe protect the store (above), too.
	s.slock(ctx, false)
	s.Facts = make(map[string]RawFact)
	s.sunlock(ctx, false)
	return err
}

func (s *LinearState) Delete(ctx *Context) error {
	Log(DEBUG, ctx, "LinearState.Delete", "name", s.Name)
	err := s.store.Delete(ctx, s.Name)
	// Maybe protect the store (above), too.
	s.slock(ctx, false)
	s.Facts = make(map[string]RawFact)
	s.sunlock(ctx, false)
	return err
}

func (s *LinearState) Get(ctx *Context, id string) (Map, error) {
	fact, err := s.get(ctx, id, true)
	return fact, err
}

func (s *LinearState) get(ctx *Context, id string, getLock bool) (Map, error) {
	Log(DEBUG, ctx, "LinearState.get", "name", s.Name, "id", id)

	if getLock {
		s.slock(ctx, true)
	}
	rf, found := s.Facts[id]
	if getLock {
		s.sunlock(ctx, true)
	}

	if !found {
		return nil, NewNotFoundError("%s", id)
	}
	expired, err := s.expire(ctx, id, rf.M, 0)
	if err != nil {
		Log(ERROR, ctx, "LinearState.Get", "error", err, "when", "expiring")
		return nil, err
	}
	if expired {
		Log(ERROR, ctx, "LinearState.Get", "expired", expired, "id", id)
		return nil, NewNotFoundError("%s", id)
	}
	maybeInjectId(ctx, id, rf.M, false)
	return rf.M, nil
}

// expire checks for expiration and removes the fact if expired.
//
// This method is mostly generic and could be dissociated from
// IndexedState.
func (s *LinearState) expire(ctx *Context, id string, fact map[string]interface{}, now int64) (bool, error) {
	// Same code for LinearState.  ToDo: Generalize.
	expired, err := checkExpiration(ctx, fact, now)
	if err != nil {
		return false, err
	}

	if expired {
		// Lots of things can go wrong and get us in an inconsistent state.
		// ToDo: Be more careful.

		Log(DEBUG, ctx, "Expire", "fact", fact, "expired", expired, "now", now)

		if _, err := s.rem(ctx, id, false); err != nil {
			Log(ERROR, ctx, "LinearState.expire", "name", s.Name,
				"when", "Rem", "error", err)
			return true, err
		}
	}

	return expired, nil
}
