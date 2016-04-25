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
	"errors"
	"fmt"
	"strings"
	"time"
	// "code.google.com/p/rog-go/exp/deepcopy" // Broken
)

type AddHookFn func(ctx *Context, state State, id string, fact Map, loading bool) error
type RemHookFn func(ctx *Context, state State, id string) error

type State interface {
	Count(ctx *Context) int
	AddHook(hook AddHookFn)
	RemHook(hook RemHookFn)
	Load(ctx *Context) error
	Add(ctx *Context, id string, x Map) (string, error)
	Rem(ctx *Context, id string) (bool, error)
	Delete(ctx *Context) error
	Get(ctx *Context, id string) (Map, error)
	get(ctx *Context, id string, getLock bool) (Map, error)
	Search(ctx *Context, pattern Map) (*SearchResults, error)
	FindRules(ctx *Context, event Map) (map[string]Map, error)
	Clear(ctx *Context) error
}

const KW_DeleteWith = "deleteWith"

// Here's a little gear to inject a fact's id into the fact itself.
// Why?  Because a rule condition might want to bind variables to ids.
//
// Using this code is optiona.  See 'IdInjectionTime'.
//
// Note that IndexedState can't easily inject ids at query time
// because the facts won't be indexed under their ids.
//
// The 'GetProp()' and 'GetIdProp()' business complicates things.
// That code is an optimization (at least in 'IndexedState'), which
// relies on particular fact forms.  That code has now changed to deal
// with optional id injection.

// KW_id is the property for an injected id.
const KW_id = "_id"

const (
	// InjectIdAtWrite will add KW_id:id to every asserted fact.
	InjectIdAtWrite = iota

	// InjectIdAtRead would, if it could be implemented easily,
	// inject add KW_id:id at query time.  This mode is not
	// supported.  Will induce panic.
	// InjectIdAtRead

	// InjectIdNever will skip id injection of any kind.
	InjectIdNever
)

// maybeInjectId might add KW_id:id to the given fact according to 'IdInjectionTime'.
//
// If it does, the function returns true; otherwise false.
func maybeInjectId(ctx *Context, id string, fact map[string]interface{}, writing bool) bool {
	switch SystemParameters.IdInjectionTime {
	case InjectIdNever:
		// Do nothing.

	case InjectIdAtWrite:
		if writing {
			existing, have := fact[KW_id]
			fact[KW_id] = id
			if have {
				previous, _ := existing.(string)
				if previous != id {
					Log(DEBUG, ctx, "maybeInjectId", "existing", existing, "id", id)
					panic("overwrite")
				} else {
					return false // No change
				}
			} else {
				Log(DEBUG, ctx, "maybeInjectId", "injected", id)
				return true // Injected
			}
		}
	}
	return false
}

// ValidateId will return an error if the id is illegal.
//
// The empty id is illegal, and any id starting with '!' is also
// illegal.  An id longer that IdLengthLimit is also illegal.  All
// other ids are (for now) legal.
func ValidateId(ctx, id string) error {
	n := len(id)
	if n == 0 {
		return fmt.Errorf("id cannot have zero length")
	}
	if SystemParameters.IdLengthLimit < n {
		return fmt.Errorf("id is too long (limit: %d)", SystemParameters.IdLengthLimit)
	}
	if id[0] == '!' {
		return fmt.Errorf("id '%s' cannot start with a '!'", id)
	}
	return nil
}

// genPropId generates the canonical fact id for a property for
// the given id and prop.
func genPropId(id string, prop string) string {
	return "!" + id + "." + prop
}

// IdProperty returns true if the given prop starts with '!'.
func IdProperty(p string) bool {
	return 0 < len(p) && p[0] == '!'
}

// idProperty prepends a '!' to the given prop (if necessary).
func idProperty(p string) string {
	if strings.HasPrefix(p, "!") {
		return p
	}
	return "!" + p
}

// GenId generates, if necessary, and id suitablef or the given fact.
//
// If the fact is a property, then the id will be canonical.
// Otherwise, if 'def' is empty, a random id is generated.  Otherwise,
// 'def' is returned.  An error can occur if a call to 'parseProp'
// returns an error.
func GenId(ctx *Context, fact map[string]interface{}, def string) (string, error) {
	isProp, id, prop, _, err := parseProp(fact)
	if err != nil {
		return "", err
	}
	if isProp {
		return genPropId(id, prop), nil
	}
	if def == "" {
		def = UUID()
	}
	return def, nil
}

// getPropFromFact generates the canonical id, gets the fact for that
// id, and returns the fact's prop value.
//
// If the fact doesn't exist, the default 'def' is returned.
//
// If the fact exists but it doesn't have the given prob, and error is
// returned.
//
// The bool return values indicated whether the value was found or the
// default was used.
func getPropFromFact(ctx *Context, s State, factId string, prop string, def interface{}) (interface{}, bool, error) {
	Log(DEBUG, ctx, "getPropFromFact", "factId", factId, "prop", prop, "factId", factId)
	fact, err := s.Get(ctx, factId)
	if _, missing := err.(*NotFoundError); missing {
		Log(DEBUG, ctx, "getPropFromFact", "missing", factId)
		return def, false, nil
	}
	if err != nil {
		Log(ERROR, ctx, "getPropFromFact", "error", err, "factId", factId)
		return def, false, err
	}
	prop = idProperty(prop)
	x, have := fact[prop]
	if !have {
		err := fmt.Errorf("internal error: missing prop '%s' in %#v", prop, fact)
		Log(WARN, ctx, "getPropFromFact", "prop", prop, "factId", factId, "warn", err)
		return def, false, err
	}
	return x, true, nil
}

// GetPropString is a wrapper around 'getProp' that returns a string value.
func GetPropString(ctx *Context, s State, prop string, def string) (string, bool, error) {
	got, found, err := getProp(ctx, s, "", prop, def)
	if err != nil {
		return def, found, err
	}
	str, ok := got.(string)
	if !ok {
		return def, found, fmt.Errorf("val %#v not a string", got)
	}
	return str, found, nil
}

// parseProp reports the fact has exactly one IdProperty.  If so,
// returns the value for the 'id' property (if any), that property,
// and the value of the property.
//
// If the fact has an 'id', the value must be a string.
//
// Examples
//
//     {"id":"h73yd7yw7sa", "!author":"homer", ...}
//     {"id":"h73yd7yw7sa", "!disabled":true, ...}
//     {"!writeKey":"ef8c6067", ...}
//
// This function will return an error if it finds multiple properties
// starting with a '!'.  This function will also return an error if
// the fact's 'id' value is not a string.
func parseProp(fact map[string]interface{}) (is bool, id string, prop string, val interface{}, err error) {
	// First check for that magic property
	found := false
	for p, v := range fact {
		if IdProperty(p) {
			if found {
				err = errors.New("more than one IdProperty")
				return
			}
			prop = p[1:]
			val = v
			found = true
		}
	}
	if !found {
		return
	}
	is = true

	x, givenId := fact["id"]
	if givenId {
		s, ok := x.(string)
		if !ok {
			err = fmt.Errorf("bad id value %#v (%T)", x, x)
			return
		}
		id = s
	}
	return
}

// getProp attempts to find the property value for the target 'id' and
// prop.
//
// Calls 'getPropFromFact' after 'genPropId'.
func getProp(ctx *Context, s State, id string, prop string, def interface{}) (interface{}, bool, error) {
	return getPropFromFact(ctx, s, genPropId(id, prop), prop, def)
}

// GetProp is the high-level property API to get the property value for
// the given target id and prop.
//
// Default value is provided by 'def'.
func GetProp(ctx *Context, s State, id string, prop string, def interface{}) (interface{}, bool, error) {
	Log(DEBUG, ctx, "GetProp", "id", id, "prop", prop)
	return getProp(ctx, s, id, prop, def)
}

// SetProp is the high-level property setter.
//
// The given id is the target id.
func SetProp(ctx *Context, s State, id string, prop string, val interface{}) (string, error) {
	Log(INFO, ctx, "SetProp", "id", id, "prop", prop, "val", val)
	prop = idProperty(prop)
	fact := Map{
		"id":          id,
		prop:          val,
		KW_DeleteWith: []interface{}{id},
	}
	return s.Add(ctx, "", fact)
}

// RemProp does what you'd think.
//
// The given id is the target id.
func RemProp(ctx *Context, s State, id string, prop string) (bool, error) {
	Log(INFO, ctx, "RemProp", "prop", prop, "id", id)
	return s.Rem(ctx, genPropId(id, prop))
}

func Expire(ctx *Context, s State, id string, fact map[string]interface{}, now int64) (bool, error) {
	expired, err := checkExpiration(ctx, fact, now)
	if err != nil {
		return false, err
	}

	if expired {
		// Lots of things can go wrong and get us in an inconsistent state.
		// ToDo: Be more careful.

		Log(DEBUG, ctx, "Expire", "fact", fact, "expired", expired, "now", now)
		if _, err := s.Rem(ctx, id); err != nil {
			Log(ERROR, ctx, "Expire", "error", err, "id", id)
			return true, err
		}
	}

	return expired, nil
}

func PrepareFact(ctx *Context, givenId string, x Map) (id string, m map[string]interface{}, err error) {
	Log(DEBUG, ctx, "PrepareFact", "givenId", givenId, "givenx", x)
	m = make(map[string]interface{})
	for p, v := range x {
		m[p] = v
	}

	id, err = GenId(ctx, m, givenId)
	if err != nil {
		Log(UERR, ctx, "PrepareFact", "givenId", givenId, "error", err)
		return
	}
	Log(DEBUG, ctx, "PrepareFact", "givenId", givenId, "id", id)

	expiring, expires, err := setExpires(ctx, m)
	if err != nil {
		Log(UERR, ctx, "PrepareFact", "givenId", "error", err)
		return
	}
	if expiring && notAfter(ctx, expires, 0) {
		err = &ExpiredError{}
		Log(UERR, ctx, "PrepareFact", "givenId", givenId, "error", err)
		return
	}

	{
		// Just for logs.  Good to know.
		ttl := expires - time.Now().UTC().Unix()
		Log(DEBUG, ctx, "PrepareFact", "givenId", givenId, "ttl", ttl)
	}

	maybeInjectId(ctx, id, m, true)

	Log(DEBUG, ctx, "PrepareFact", "givenId", givenId, "id", id, "x", m)

	return
}

// SearchResult packages up a found fact and the bindings that make it
// match the query.
type SearchResult struct {
	Js        string
	Id        string
	Bindingss []Bindings
}

// SearchResults packages up what a search found.  When we search, we
// should remember how many candidate facts we had to test (Checked)
// in order to find actual matches.  Part of the efficiency of
// extractTerms and the TermIndex is indicated by the number of false
// positives (Checked - len(Found)).  This metric will be important.
type SearchResults struct {
	Found   []SearchResult
	Checked int
	Elapsed int64
	Expired int
}

func (sr *SearchResults) Merge(more *SearchResults) *SearchResults {
	sr.Found = append(sr.Found, more.Found...)
	sr.Checked += more.Checked
	sr.Elapsed += more.Elapsed
	sr.Expired += more.Expired
	return sr
}

// setExpires looks for a given 'ttl' or 'expires' property,
// canonicalizes the value, and returns a rewritten Javascript
// representation of the fact, the expiration time (UNIX seconds), or
// an error.
//
// A 0 expiration time means no expiration.
//
// If the fact defines a rule, then the 'expires' property is also
// written inside that rule.
func setExpires(ctx *Context, fact map[string]interface{}) (bool, int64, error) {
	Log(DEBUG, ctx, "State.setExpires", "fact", fact)
	var expires int64

	// If "ttl", compute "expires" and removes "ttl".
	if ttl, given := fact["ttl"]; given {
		Log(DEBUG, ctx, "setExpires", "ttl", ttl, "fact", fact)
		delete(fact, "ttl")
		// TTL can be a string (duration) or a number (seconds).
		switch vv := ttl.(type) {
		case float64: // Only kind of number in Javascript!
			expires = NowSecs() + int64(vv)
		case int64:
			expires = vv
		case string:
			d, err := time.ParseDuration(vv)
			if err != nil {
				return true, expires, err
			}
			expires = time.Now().Add(d).UTC().Unix()
		default:
			err := fmt.Errorf("bad TTL %v (%T)", ttl, ttl)
			Log(ERROR|USR, ctx, "setExpires", "error", err)
			return true, expires, err
		}
		fact["expires"] = expires
	}

	hasExpiration := false
	if exp, given := fact["expires"]; given {
		hasExpiration = true
		Log(DEBUG, ctx, "expires", "expires", exp)
		// 'Expires' should be a string in RFC3339 syntax or a
		// number representing the time (UNIX seconds).
		switch vv := exp.(type) {
		case float64: // Only kind of number in Javascript!
			expires = int64(vv)
		case int64: // Can arrive from Go (via, say, a test case)
			expires = vv
		case string:
			t, err := time.Parse(time.RFC3339, vv)
			if err != nil {
				return true, expires, err
			}
			expires = t.UTC().Unix()
			fact["expires"] = expires
		default:
			err := fmt.Errorf("Expected a string or number for expires, not %v (%T)", exp, exp)
			Log(ERROR|USR, ctx, "setExpires", "error", err)
			return true, expires, err
		}

		if rule, given := fact["rule"]; given {
			if r, ok := rule.(map[string]interface{}); ok {
				r["expires"] = expires
			} else {
				return false, expires, errors.New("'rule' isn't a rule")
			}
		}
	}

	Log(DEBUG, ctx, "State.setExpires", "fact", fact, "hasExpiration", hasExpiration, "expires", expires)

	return hasExpiration, expires, nil
}

// getExpiration looks for 'expires' property and returns it as an int64.
//
// Value should be in UNIX seconds.
func getExpiration(ctx *Context, fact map[string]interface{}) (int64, error) {
	then, given := fact["expires"]
	if given {
		switch vv := then.(type) {
		case int64:
			return vv, nil
		case float64:
			return int64(vv), nil
		default:
			err := fmt.Errorf("bad 'expires' %#v (%T)", then, then)
			Log(WARN, ctx, "getExpiration", "error", err)
			return 0, err
		}
	}
	return 0, nil
}

func notAfter(ctx *Context, secs int64, then int64) bool {
	if secs == 0 {
		return false
	}
	if then == 0 {
		then = time.Now().UTC().Unix()
	}
	Log(DEBUG, ctx, "notAfter", "delta", secs-then, "then", then, "secs", secs)
	return secs <= then
}

// checkExpires determines if the given fact has expired.
//
// Assumes that 'setExpires()' was called previous so that we have a
// good 'expires' value if any.
func checkExpiration(ctx *Context, fact map[string]interface{}, unixNow int64) (bool, error) {
	Log(DEBUG, ctx, "checkExpiration", "fact", fact)
	expires, err := getExpiration(ctx, fact)
	if err != nil {
		Log(ERROR, ctx, "checkExpiration", "fact", fact, "error", err)
		return false, err
	}
	Log(DEBUG, ctx, "checkExpiration", "fact", fact, "expires", expires, "ttl", expires-NowSecs())
	if notAfter(ctx, expires, unixNow) {
		Log(DEBUG, ctx, "checkExpiration", "expired", true, "fact", fact)
		return true, nil
	}
	Log(DEBUG, ctx, "checkExpiration", "expired", false, "fact", fact)
	return false, nil
}

// Rules are mostly made to be broken and are too often for the lazy
// to hide behind.
//
//  --Douglas MacArthur, reported in William A. Ganoe's MacArthur Close-Up
