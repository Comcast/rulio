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

// A location's state, which is basically a write-through cache in
// front of persistent storage.  A location stores facts and rules.
// This code stores rules as facts ('{"rule":{"when":...}}').  Facts
// are indexed with a TermIndex.  To find rules given an event, we use
// a PatternIndex to index rule 'when' clauses.

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	// "code.google.com/p/rog-go/exp/deepcopy" // Broken
)

type IndexedState struct {
	sync.RWMutex

	// Name should probably the name of the location for this state.
	Name string

	// Loc points to the state's location in order to call back to
	// the location for certain operations.
	loc *Location

	// FactIndex is the in-memory term index that allows us to
	// search facts.
	FactIndex *TermIndex

	// RuleIndex is the in-memory pattern index that allows us to
	// find rules for an incoming event.
	RuleIndex *PatternIndex

	// Map from IDs to facts.
	IdToFact map[string]Map

	// Store is how we persist and load data.
	Store Storage

	// Loaded indicates whether we have loaded data from Store.
	Loaded bool

	addHook AddHookFn

	remHook RemHookFn
}

func (s *IndexedState) withPrivilege(ctx *Context) {
	ctx.grantPrivilege("hook")
}

func (s *IndexedState) withoutPrivilege(ctx *Context) {
	ctx.revokePrivilege()
}

func (s *IndexedState) slock(ctx *Context, read bool) {
	Log(DEBUG, ctx, "IndexedState.slock", "read", read, "priv", ctx.isPrivileged("hook"))
	if ctx != nil && ctx.isPrivileged("hook") {
		return
	}
	if read {
		s.RLock()
	} else {
		s.Lock()
	}
}

func (s *IndexedState) sunlock(ctx *Context, read bool) {
	Log(DEBUG, ctx, "IndexedState.sunlock", "read", read, "priv", ctx.isPrivileged("hook"))
	if ctx != nil && ctx.isPrivileged("hook") {
		return
	}
	if read {
		s.RUnlock()
	} else {
		s.Unlock()
	}
}

func (s *IndexedState) init(ctx *Context) error {
	s.IdToFact = make(map[string]Map)
	s.FactIndex = NewTermIndex()
	s.RuleIndex = NewPatternIndex()
	return nil
}

func NewIndexedState(ctx *Context, name string, store Storage) (*IndexedState, error) {
	s := IndexedState{}
	s.Name = name
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	s.Store = store
	return &s, nil
}

func (s *IndexedState) AddHook(hook AddHookFn) {
	s.addHook = hook
}

func (s *IndexedState) RemHook(hook RemHookFn) {
	s.remHook = hook
}

func (s *IndexedState) Count(ctx *Context) int {
	s.slock(ctx, true)
	n := len(s.IdToFact)
	s.sunlock(ctx, true)
	return n
}

func (s *IndexedState) IsLoaded(ctx *Context) bool {
	s.slock(ctx, true)
	loaded := s.Loaded
	s.sunlock(ctx, true)
	return loaded
}

func (s *IndexedState) Load(ctx *Context) error {
	Log(INFO, ctx, "IndexedState.Load", "location", s.Name)
	timer := NewTimer(ctx, "IndexedState.Load")
	// ToDo: Combine defered code?
	defer timer.Stop()

	s.slock(ctx, false)
	defer s.sunlock(ctx, false)

	if s.Loaded {
		Log(WARN, ctx, "IndexedState.Load", "location", s.Name, "loaded", "already")
		return nil
	}

	pairs, err := s.Store.Load(ctx, s.Name)
	if err != nil {
		Log(ERROR, ctx, "IndexedState.Load", "location", s.Name, "error", err, "when", "Store.Load")
		return err
	}
	for _, pair := range pairs {
		id := string(pair.K)
		bs := pair.V
		var x Map
		if err := json.Unmarshal(bs, &x); err != nil {
			return err
		}
		_, err := s.add(ctx, id, x)
		if err != nil {
			_, is := err.(*ExpiredError)
			if is {
				// We have an expired fact in storage.
				// Need to delete it and then skip it here.
				// since no cache has been created, just remove it directly from the store
				if _, err = s.Store.Remove(ctx, s.Name, []byte(id)); err != nil {
					Log(ERROR, ctx, "IndexedState.Load", "location", s.Name, "error", err, "when", "rem", "id", id)
					return err
				}
			} else {
				Log(ERROR, ctx, "IndexedState.Load", "location", s.Name, "error", err, "when", "Store.Add", "pair", pair)
				return err
			}
		}
	}

	s.Loaded = true
	return nil
}

// ExtractTerms gets the terms from the given fact (or pattern).
// Since our TermIndex currently uses hash tables, it cannot do
// searches based on prefixes.  Therefore, we extract all atomic terms
// without regard to structure.
//
// See the Elasticsearch FDS in another repo.  That FDS uses Lucene's
// B*trees ordering to do prefix-based searches.  If we are using such
// an FDS, we could extract terms with structure such as 'a.b.c' from
// {"a":{"b":"c"}}.  We went to a fair amount of trouble to do that
// previously, and we might want to reimplement this FDS to provide
// support for searches based on term prefixes.  Shouldn't be a huge
// deal.
//
// We might want to skip certain terms here that will have little
// value.  Large numbers are candidates.  Will a large number really
// help to find facts?  We don't want an unbounded number of terms
// anyway, so numbers are suspect.
func ExtractTerms(ctx *Context, fact map[string]interface{}) []string {
	terms := make(StringSet)
	extractTermsAux(ctx, fact, terms, 0)
	acc := terms.Array()
	Log(DEBUG, ctx, "core.ExtractTerms", "fact", fact, "terms", acc)
	return acc
}

func extractTermsAux(ctx *Context, x interface{}, terms StringSet, depth int) {
	switch vv := x.(type) {
	case string:
		if !IsVariable(vv) {
			if len(vv) < SystemParameters.StringLengthTermLimit {
				terms.Add(vv)
			}
		}
	case map[string]interface{}: // Fact
		for k, v := range vv {
			// Always index the property.
			extractTermsAux(ctx, k, terms, depth+1)
			switch k {
			case "rule":
				// Don't index the body of a rule.
				continue
			}
			// Do not index the value of a property ending in '!'.
			if strings.HasSuffix(k, "!") {
				continue
			}
			extractTermsAux(ctx, v, terms, depth+1)
		}
	case []interface{}:
		for _, y := range vv {
			extractTermsAux(ctx, y, terms, depth+1)
		}
	case []string:
		// ToDo: Contemplate use of ISlice.
		for _, s := range vv {
			extractTermsAux(ctx, s, terms, depth+1)
		}
	default:
		// We don't index what we don't understand -- or what
		// we otherwise choose to ignore.  Numbers, for
		// example.
	}
	Log(DEBUG, ctx, "core.extractTermsAux", "input", Gorep(x), "terms", terms)
}

func (s *IndexedState) Add(ctx *Context, id string, x Map) (string, error) {
	Log(DEBUG, ctx, "IndexedState.Add", "state", s.Name, "factx", x, "id", id)
	s.slock(ctx, false)
	id, err := s.add(ctx, id, x)
	s.sunlock(ctx, false)

	if nil != err {
		return "", err
	}

	js, err := json.Marshal(&x)
	if err != nil {
		return "", err
	}
	d := Pair{[]byte(id), js}

	err = s.Store.Add(ctx, s.Name, &d)
	if err != nil {
		Log(WARN, ctx, "IndexedState.Add", "state", s.Name, "factjs", string(js), "id", id, "error", err)
		return "", err
	}
	return id, err
}

func (s *IndexedState) add(ctx *Context, id string, x Map) (string, error) {
	Log(DEBUG, ctx, "IndexedState.add", "state", s.Name, "factx", x, "id", id)
	then := time.Now()

	id, fact, err := PrepareFact(ctx, id, x)
	if err != nil {
		return id, err
	}

	rule, err := ExtractRule(ctx, fact, false)
	if err != nil {
		return id, err
	}
	if rule != nil {
		// ToDo: Metric(ctx, "RuleUpdated", "location", s.Name, "ruleId", id)
		Log(DEBUG, ctx, "IndexedState.add", "state", s.Name, "rule", rule, "ruleId", id)
		if _, scheduled := rule["schedule"]; !scheduled {
			if err = s.indexRule(ctx, id, rule); err != nil {
				return "", err
			}
		}
	}

	// Try the hook first?
	if s.addHook != nil {
		s.withPrivilege(ctx)
		defer s.withoutPrivilege(ctx)
		err := s.addHook(ctx, s, id, fact, ctx.GetLoc().loading)
		if err != nil {
			Log(ERROR, ctx, "IndexedState.add", "state", s.Name, "error", err,
				"when", "addHook")
			return "", err
		}
	}

	Log(DEBUG, ctx, "IndexedState.add", "state", s.Name, "fact", fact)
	terms := ExtractTerms(ctx, fact)
	for _, term := range terms {
		s.FactIndex.Add(ctx, term, id)
	}

	s.IdToFact[id] = fact

	elapsed := time.Now().Sub(then).Nanoseconds()
	Log(DEBUG, ctx, "IndexedState.add", "state", s.Name, "id", id, "elapsed", elapsed)
	return id, nil
}

// GetRulesPatterns extracts the rule's 'when' pattern.
func GetRulePatterns(ctx *Context, rule map[string]interface{}) []map[string]interface{} {
	// Never should have allowed this kind of thing.
	eventPattern, have := rule["when"]
	if !have {
		return nil
	}
	when := eventPattern.(map[string]interface{})
	events := make([]map[string]interface{}, 0, 1)
	p, fromQuery := when["pattern"]
	// ToDo: Better type processing.
	if fromQuery {
		events = append(events, p.(map[string]interface{}))
	} else {
		ps, fromQuery := when["patterns"]
		if fromQuery {
			for _, p := range ps.([]interface{}) {
				events = append(events, p.(map[string]interface{}))
			}
		} else {
			events = append(events, when)
		}
	}
	Log(DEBUG, ctx, "core.GetRulePatterns", "events", events)
	return events
}

func (s *IndexedState) indexRule(ctx *Context, id string, rule map[string]interface{}) error {
	Log(DEBUG, ctx, "IndexedState.indexRule", "state", s.Name, "rule", rule, "ruleId", id)
	patterns := GetRulePatterns(ctx, rule)
	if nil == patterns {
		return NewSyntaxError("No 'when' in rule.")
	}

	_, have := s.IdToFact[id]
	if have {
		if err := s.unindexRule(ctx, id, rule); err != nil {
			return err
		}
	}

	for _, m := range patterns {
		if err := s.RuleIndex.AddPatternMap(ctx, m, id); err != nil {
			Log(ERROR, ctx, "IndexedState.indexRule", "id", id, "pattern", m, "error", err)
			return err
		}
	}
	return nil
}

func (s *IndexedState) unindexRule(ctx *Context, id string, rule map[string]interface{}) error {
	Log(DEBUG, ctx, "IndexedState.unindexRule", "state", s.Name, "rule", rule, "ruleId", id)
	patterns := GetRulePatterns(ctx, rule)
	if patterns == nil {
		return nil
	}

	for _, m := range patterns {
		Log(DEBUG, ctx, "IndexedState.index", "id", id, "pattern", m)
		if err := s.RuleIndex.RemPatternMap(ctx, m, id); err != nil {
			Log(ERROR, ctx, "IndexedState.unindex", "id", id, "pattern", m, "error", err)
			return err
		}
	}
	return nil
}

func (s *IndexedState) Rem(ctx *Context, id string) (bool, error) {
	Log(DEBUG, ctx, "IndexedState.Rem", "id", id)
	timer := NewTimer(ctx, "IndexedState.Rem")
	defer timer.Stop()
	s.slock(ctx, false)
	defer s.sunlock(ctx, false)
	if s.remHook != nil {
		// Consider the lock.
		s.withPrivilege(ctx)
		defer s.withoutPrivilege(ctx)
		err := s.remHook(ctx, s, id)
		if err != nil {
			Log(ERROR, ctx, "IndexedState.Rem", "state", s.Name, "error", err,
				"id", id, "when", "remHook")
			// ToDo: Consider queueing, falling through, ...
			return false, err
		}
	}
	done, err := s.rem(ctx, id)
	return done, err
}

func (s *IndexedState) rem(ctx *Context, id string) (bool, error) {
	Log(DEBUG, ctx, "IndexedState.rem", "name", s.Name, "id", id)

	// Currently we don't return an error if the fact isn't found.
	// ToDo: Reconsider.  For example, maybe have an additional
	// argument that controls whether an error is return when the
	// fact isn't found.

	fact, have := s.IdToFact[id]
	if have {
		Log(DEBUG, ctx, "IndexedState.rem", "state", s.Name, "id", id, "fact", fact)

		rule, err := ExtractRule(ctx, fact, false)
		if err != nil {
			return false, nil
		}

		if rule != nil {
			if err := s.unindexRule(ctx, id, rule); err != nil {
				return false, err
			}
		}

		delete(s.IdToFact, id)

		s.FactIndex.RemIdTerms(ctx, ExtractTerms(ctx, fact), id)

		_, err = s.Store.Remove(ctx, s.Name, []byte(id))
		if err != nil {
			return true, err
		}
	} else {
		Log(DEBUG, ctx, "IndexedState.rem", "state", s.Name, "id", id, "warning", "not found")
	}

	// ToDo: contemplate performance impact.
	if err := s.deleteDependencies(ctx, id); nil != err {
		// ToDo: Somebody handle err
		Log(ERROR, ctx, "IndexedState.rem", "state", s.Name, "id", id, "error", err, "when", "deleteDependencies")
		return have, err
	}

	return have, nil
}

func (s *IndexedState) deleteDependencies(ctx *Context, id string) error {
	Log(DEBUG, ctx, "IndexedState.deleteDependencies", "location", s.Name, "id", id)
	srs, err := s.search(ctx, Map{KW_DeleteWith: []string{id}})
	if nil != err {
		return err
	}
	Log(DEBUG, ctx, "IndexedState.deleteDependencies", "location", s.Name, "id", id, "found", len(srs.Found))

	for _, sr := range srs.Found {
		Log(DEBUG, ctx, "IndexedState.deleteDependencies",
			"location", s.Name, "id", id, "target", sr.Id)
		if _, err := s.rem(ctx, sr.Id); nil != err {
			return err
		}
	}

	return nil
}

func (s *IndexedState) remHooks(ctx *Context) error {
	if s.remHook != nil {
		// Try to run the remHook for every fact.
		//
		// Consider the lock.
		s.withPrivilege(ctx)
		defer s.withoutPrivilege(ctx)
		for id, _ := range s.IdToFact {
			err := s.remHook(ctx, s, id)
			if err != nil {
				Log(ERROR, ctx, "IndexedState.Clear", "state", s.Name, "error", err,
					"id", id, "when", "remHook")
				return err
			}
		}
	}
	return nil
}

func (s *IndexedState) Clear(ctx *Context) error {
	// We can easily get inconsistent here due to various failures.
	// ToDo: Something better.

	Log(INFO, ctx, "IndexedState.Clear", "name", s.Name)
	s.slock(ctx, false)
	defer s.sunlock(ctx, false)

	if err := s.remHooks(ctx); err != nil {
		return err
	}

	var err error
	if _, err = s.Store.Clear(ctx, s.Name); err == nil {
		err = s.init(ctx)
	}
	// ToDo: Something better
	return err
}

func (s *IndexedState) Delete(ctx *Context) error {
	Log(DEBUG, ctx, "IndexedState.Delete", "name", s.Name)
	s.slock(ctx, false)
	defer s.sunlock(ctx, false)
	if err := s.remHooks(ctx); err != nil {
		return err
	}
	var err error
	if err = s.Store.Delete(ctx, s.Name); err == nil {
		err = s.init(ctx)
	}
	// ToDo: Something better
	return err
}

func (s *IndexedState) Get(ctx *Context, id string) (Map, error) {
	fact, err := s.get(ctx, id, true)
	return fact, err
}

func (s *IndexedState) get(ctx *Context, id string, getLock bool) (Map, error) {
	// Assumes we have a read lock!
	Log(DEBUG, ctx, "IndexedState.get", "name", s.Name, "id", id)

	if getLock {
		s.slock(ctx, true)
	}
	fact, found := s.IdToFact[id]
	if getLock {
		s.sunlock(ctx, true)
	}

	if !found {
		return nil, NewNotFoundError("%s", id)
	}

	expired, err := s.expire(ctx, id, fact, 0)
	if err != nil {
		Log(ERROR, ctx, "IndexedState.Get", "error", err, "when", "expiring")
		return nil, err
	}
	if expired {
		Log(ERROR, ctx, "IndexedState.Get", "expired", expired, "id", id)
		return nil, NewNotFoundError("%s", id)
	}

	maybeInjectId(ctx, id, fact, false)
	return fact, nil
}

func (s *IndexedState) SearchForIDs(ctx *Context, pattern Map) ([]string, error) {
	Log(DEBUG, ctx, "IndexedState.SearchForIDs", "location", s.Name, "pattern", pattern)
	terms := ExtractTerms(ctx, pattern)

	ids, err := s.FactIndex.Search(ctx, terms)

	return ids, err
}

// expire checks for expiration and removes the fact if expired.
//
// This method is mostly generic and could be dissociated from
// IndexedState.
func (s *IndexedState) expire(ctx *Context, id string, fact map[string]interface{}, unixNow int64) (bool, error) {
	Log(DEBUG, ctx, "IndexedState.expire", "id", id, "fact", fact)

	// Same code for LinearState.  ToDo: Generalize.
	expired, err := checkExpiration(ctx, fact, unixNow)
	if err != nil {
		return false, err
	}

	if expired {
		// Lots of things can go wrong and get us in an inconsistent state.
		// ToDo: Be more careful.

		Log(DEBUG, ctx, "IndexedState.expire", "fact", fact, "expired", expired, "now", unixNow)

		if _, err := s.rem(ctx, id); err != nil {
			Log(ERROR, ctx, "IndexedState.expire", "name", s.Name,
				"when", "Rem", "error", err)
			return true, err
		}
	}

	return expired, nil
}

// Search queries the term index for the given pattern (represented as JSON).
// Obtains and releases the FDS mutex.
func (s *IndexedState) Search(ctx *Context, pattern Map) (*SearchResults, error) {
	timer := NewTimer(ctx, "IndexedState.Search")
	defer timer.Stop()

	s.slock(ctx, true)
	srs, err := s.search(ctx, pattern)
	s.sunlock(ctx, true)

	return srs, err
}

func (s *IndexedState) search(ctx *Context, pattern Map) (*SearchResults, error) {
	Log(DEBUG, ctx, "IndexedState.search", "pattern", pattern)
	then := Now()

	ids, err := s.SearchForIDs(ctx, pattern)
	if err != nil {
		return nil, err
	}

	sra := make([]SearchResult, 0, len(ids)) // ToDo: More dynamic
	expired := 0
	now := NowSecs()
	for _, id := range ids {

		fact, ok := s.IdToFact[id]
		if !ok {
			Log(ERROR, ctx, "IndexedState.search", "warning", "missing JSON", "id", id)
			continue
		}

		done, err := s.expire(ctx, id, fact, now)
		if err != nil {
			Log(ERROR, ctx, "IndexedState.search", "error", err, "when", "expiring")
		}
		if done {
			Log(ERROR, ctx, "IndexedState.search", "expired", done, "id", id)
			expired++
			continue
		}

		bss, err := Matches(ctx, pattern, map[string]interface{}(fact))
		if err != nil {
			return nil, err
		}
		if 0 < len(bss) {
			maybeInjectId(ctx, id, fact, false)
			js, err := json.Marshal(&fact)
			if err != nil {
				Log(ERROR, ctx, "IndexedState.search", "error", err,
					"when", "marshal", "fact", fact)
				return nil, err
			}
			// ToDo: Eliminate JSON
			sr := SearchResult{string(js), id, bss}
			sra = append(sra, sr)
		}
	}
	srs := SearchResults{sra, len(ids), Now() - then, expired}

	Log(DEBUG, ctx, "IndexedState.search", "numIds", len(ids), "srs", sra)

	if ctx != nil && ctx.GetLoc() != nil {
		c := ctx.GetLoc().Control()
		if c != nil {
			limit := c.BindingsWarningLimit
			if 0 < limit && limit < len(srs.Found) {
				Log(WARN, ctx, "IndexedState.search", "name", s.Name,
					"bindingsWarning", len(srs.Found),
					"limit", limit)
			}
		}
	}

	return &srs, nil
}

func (s *IndexedState) FindRules(ctx *Context, event Map) (map[string]Map, error) {
	Log(DEBUG, ctx, "IndexedState.FindRules", "name", s.Name, "event", event)
	timer := NewTimer(ctx, "IndexedState.FindRules")
	defer timer.Stop()

	s.slock(ctx, true)
	defer s.sunlock(ctx, true)

	acc := make(map[string]Map)
	ss, err := s.RuleIndex.SearchPatternsMap(ctx, map[string]interface{}(event))
	if err != nil {
		Log(ERROR, ctx, "IndexedState.FindRules", "error", err)
		return nil, err
	}
	ids := ss.Array()
	now := NowSecs()

	for _, id := range ids {
		rule, ok := s.IdToFact[id]
		Log(DEBUG, ctx, "IndexedState.FindRules", "rule", rule, "ruleId", id)

		expired, err := s.expire(ctx, id, rule, now)
		if err != nil {
			Log(ERROR, ctx, "IndexedState.FindRules", "error", err, "when", "expiring")
		}
		if expired {
			Log(ERROR, ctx, "IndexedState.FindRules", "expired", expired, "ruleId", id, "rule", rule)
			continue
		}

		if !ok {
			err = fmt.Errorf("lost rule with id %s", id)
			Log(ERROR, ctx, "IndexedState.FindRules", "error", err, "id", id)
			// Should we totally fail?
			return nil, err
		}

		body, err := ExtractRule(ctx, rule, true)
		if err != nil {
			Log(ERROR, ctx, "IndexedState.FindRules", "error", err)
			// Should we totally fail?
			return nil, err
		}

		// The following doesn't work, but it doesn't hurt.
		// See comments near maybeInjectId().
		// ToDo: Support read-time id injection?
		//
		// maybeInjectId(ctx, id, body, false)

		acc[id] = body
	}
	Log(DEBUG, ctx, "IndexedState.FindRules", "rules", acc)
	return acc, nil
}
