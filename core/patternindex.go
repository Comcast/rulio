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
	"sort"
	"strconv"
	"strings"
)

// PatternIndex is the state for our in-memory pattern store.
//
// It indexes patterns.  When given an event (or fact), this index
// should find candidates for matching patterns, which are typically
// the 'when's in rules.
//
// Important: This code does NOT need to return only those patterns
// that match the input.  Instead, this code needs to return all
// patterns that might match the input.  Then other code
// (state_indexed.go) will process the results based on actual pattern
// matching.
//
// The basic approach here: We have an ordering on all atomic values
// (strings, numbers), and we build a tree using that order.  When
// given a fact, we start with lowest property.  We look for that
// property in the tree.  Say we find it at some node.  Then we look
// at the fact's value at that property, and we look for that value in
// the node.  Then we get the fact's next property, and we start
// looking at the same node.  When we encounter a structured
// (non-atomic) value, we traverse a Map node instead of regular
// value.
//
// That description is pretty confusing.  See 'patstore_test.go' and
// add some 'i.Show()' calls to see how the index gets built.
//
type PatternIndex struct {
	String map[string]*PatternIndex
	Var    *PatternIndex
	Map    *PatternIndex
	Ids    StringSet
}

// NewPatternIndex does exactly what you think it does.
func NewPatternIndex() *PatternIndex {
	return &PatternIndex{nil, nil, nil, NewStringSet(nil)}
}

// The basic algorithm decomposes maps (perhaps with map values) into
// pairs.  See 'mapToPairs()'.
type piPair struct {
	key string
	val interface{}
}

// mapToPairs generates an array of key/value Pairs in the map.  We
// need to order the pairs and be able to write recursive functions to
// consume them.
func mapToPairs(ctx *Context, m map[string]interface{}) []piPair {
	keys := make([]string, len(m))
	i := 0
	for k, _ := range m {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	pairs := make([]piPair, len(keys))
	i = 0
	for _, k := range keys {
		pairs[i] = piPair{k, (m)[k]} // ISlice()
		i++
	}
	return pairs
}

// We support two write operations.
// "piOp" = "pattern index operation".
type piOp int

const (
	opAdd piOp = iota
	opRem
)

func (index *PatternIndex) add(ctx *Context, pairs []piPair, id string) error {
	return index.mod(ctx, pairs, id, opAdd)
}

func (index *PatternIndex) rem(ctx *Context, pairs []piPair, id string) error {
	return index.mod(ctx, pairs, id, opRem)
}

func picast(ctx *Context, x interface{}) interface{} {
	// fmt.Printf("picast %v %T\n", x, x)

	switch vv := x.(type) {
	case bool:
		return "B_" + strconv.FormatBool(vv)
	case float64:
		return "F_" + strconv.FormatFloat(vv, 'f', -1, 64)
	case int:
		// Sadly, we'll follow Javascript.
		return "F_" + strconv.FormatFloat(float64(vv), 'f', -1, 64)
	case string:
		if strings.HasPrefix(vv, "?") {
			return vv
		}
		if strings.HasPrefix(vv, "F_") {
			return vv
		}
		if strings.HasPrefix(vv, "B_") {
			return vv
		}
		if strings.HasPrefix(vv, "S_") {
			return vv
		}
		return "S_" + vv
	case nil:
		// This pattern index should return a superset of
		// possible matches.  It's okay to return something
		// that doesn't turn out to match, but it's not okay
		// to fail to return something that would match.
		//
		// So we conflate "null" and nil in order to support
		// queries and patterns with null values.
		Log(DEBUG, ctx, "picast", "null", "conflagration")
		return "null"
	default:
		return x
	}
}

// ThingSlice is a slice of interfaces.
//
// This type exists because we need to sort homogeneous arrays with
// various element types, and we don't want to copy data.  Maybe there
// is a better way.  Callers should check IsHomogeneous() before
// calling sort.Sort().
type ThingSlice []interface{}

func AsThingSlice(xs []interface{}) (ThingSlice, error) {
	if !IsSortable(xs) {
		return nil, fmt.Errorf("slice %#v is not sortable", xs)
	}
	return ThingSlice(xs), nil
}

func typeCode(x interface{}) int {
	switch x.(type) {
	case string:
		return 1
	case float64:
		return 2
	case int:
		return 3
	case bool:
		return 4
	default:
		return 0
	}
}

func IsSortable(xs []interface{}) bool {
	if len(xs) <= 1 {
		return true
	}
	kind := typeCode(xs[0])
	if kind == 0 {
		return false
	}
	for _, v := range xs[1:] {
		if kind != typeCode(v) {
			return false
		}
	}
	return true
}

func (a ThingSlice) Len() int {
	return len(a)
}
func (a ThingSlice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Less provides a somewhat generic comparison.
//
// Callers should check IsSortable() before invoking this method.
// If the given ThingSlice is heterogeneous, this method will panic.
//
// Also see 'IsAtomic()'.
func (a ThingSlice) Less(i, j int) bool {
	switch a[i].(type) {
	case string:
		return a[i].(string) < a[j].(string)
	case float64:
		return a[i].(float64) < a[j].(float64)
	case int:
		// Unlikely to get here because all Javascript numbers are floats!
		return a[i].(int) < a[j].(int)
	default:
		Log(ERROR, nil, "ThingSlice.Less", "error", "unsupported type", "things", a)
		return false
	}
}

// SortValues attempts to sort the slice generically.
func SortValues(vs []interface{}) ([]interface{}, error) {
	if len(vs) <= 1 {
		return vs, nil
	}
	ts, err := AsThingSlice(vs)
	if err != nil {
		return nil, err
	}
	sort.Sort(ts)
	return []interface{}(ts), nil
}

// Mod (for modification) is the primary write/update function.
//
// The method calls itself recursively to modify the index.  Users
// will call AddPatternMap, RemPatternMap, and AddPatternJSON instead
// of calling this function directly.
func (index *PatternIndex) mod(ctx *Context, pairs []piPair, id string, op piOp) error {
	// Log(INFO, ctx, "PatternIndex.mod", "pairs", pairs, "id", id)
	if len(pairs) == 0 {
		switch op {
		case opAdd:
			if index.Ids == nil {
				// Probably won't get here due to previous initialization.
				index.Ids = EmptyStringSet()
			}
			index.Ids.Add(id)
		case opRem:
			if index.Ids != nil {
				index.Ids.Rem(id)
			}
		}
		return nil
	}

	pair := pairs[0]
	rest := pairs[1:]

	k := pair.key
	v := pair.val

	var ki *PatternIndex

	if strings.HasPrefix(k, "?") {
		if !AllowPropertyVariables {
			return fmt.Errorf("Can't have variable keys like %s", k)
		}
		// An anonymous variable.
		k = "?"
	}
	si := index.String
	if si == nil {
		// ToDo: Probably not this.
		si = make(map[string]*PatternIndex)
		index.String = si
	}
	_, have := si[k]
	if !have {
		// fmt.Printf("creating key node %s\n", k)
		si[k] = NewPatternIndex()
	}
	ki = si[k]

	v = picast(ctx, v)

	switch vv := v.(type) {
	case string:
		if strings.HasPrefix(vv, "?") {
			vi := ki.Var
			if vi == nil {
				// fmt.Printf("creating var node\n")
				vi = NewPatternIndex()
				ki.Var = vi
			}
			index = vi
		} else {
			si := ki.String
			if si == nil {
				// fmt.Printf("creating string map node\n")
				si = make(map[string]*PatternIndex)
				ki.String = si
			}
			_, have := si[vv]
			if !have {
				// fmt.Printf("creating string node %s\n", vv)
				si[vv] = NewPatternIndex()
			}
			index = si[vv]
		}

	case Map, map[string]interface{}:
		// We support Fact so we can use getFact in benchmarks.

		// fmt.Printf("working map %v at %v (index %p)\n", vv, k, index)
		mi := ki.Map
		if mi == nil {
			// fmt.Printf("creating map node at %p\n", ki)
			mi = NewPatternIndex()
			ki.Map = mi
		}
		index = mi
		var mp map[string]interface{}
		switch v.(type) {
		case Map:
			mp = (map[string]interface{})(v.(Map))
		case map[string]interface{}:
			mp = vv.(map[string]interface{})
		}

		// mapPairs := mapToPairs(ctx, &vv)
		mapPairs := mapToPairs(ctx, mp)
		rest = append(mapPairs, rest...)

	case []interface{}:
		// If we have "b":[1,2], then we require the query
		// have the same.  We implement this behavior by
		// making additional branches for the property that
		// has the array as a value.
		morePairs := make([]piPair, 0, len(vv))
		// fmt.Printf("working array %v\n", vv)
		sorted, err := SortValues(vv)
		if err != nil {
			return err
		}
		for _, x := range sorted {
			var xPair piPair
			xPair.key = k
			xPair.val = picast(ctx, x)
			morePairs = append(morePairs, xPair)
		}
		rest = append(morePairs, rest...)
	default:
		return fmt.Errorf("can't handle (mod) %v (%T)", v, v)
	}

	return index.mod(ctx, rest, id, op)
}

// searchPairs is the core index reader.
//
// This method calls itself recursively to traverse the index.  Users
// will call SearchPatternsMap and SearchPatternsJSON instead of
// calling this method directly.
func (index *PatternIndex) searchPairs(ctx *Context, pairs []piPair) (StringSet, error) {
	Log(DEBUG, ctx, "PatternIndex.searchPairs", "default_input", pairs)

	if len(pairs) == 0 {
		return make(StringSet), nil
	}

	pair := pairs[0]
	rest := pairs[1:]

	k := pair.key
	v := pair.val

	if strings.HasPrefix(k, "?") {
		if AllowPropertyVariables {
			if 1 < len(pairs) {
				return nil, fmt.Errorf("Can't have variable key (%s) with other keys", k)
			}
		} else {
			return nil, fmt.Errorf("Can't have variable keys like %s", k)
		}
	}

	// Step via the key.
	si := index.String
	if si == nil {
		// Key not here.  Try next pair.
		return index.searchPairs(ctx, rest)
	}
	ki, have := si[k]
	if !have {
		if !AllowPropertyVariables {
			// Key not here.  Try next pair.
			return index.searchPairs(ctx, rest)
		}
		// Check for anonymous variable.
		if ki, have = si["?"]; !have {
			// Key not here.  Try next pair.
			return index.searchPairs(ctx, rest)
		}
	}
	// We took a step down.

	// Let's see if we can find some Ids considering the value.
	ids := make(StringSet)
	// We'll need to remember continuations.
	next := make([]*PatternIndex, 0, 0)
	next = append(next, index)

	// First check for Variable.
	vi := ki.Var
	if vi != nil {
		// Yes, there is one.
		ids.AddAll(vi.Ids)
		next = append(next, vi)
	}

	v = picast(ctx, v)

	switch vv := v.(type) {
	case string:
		if strings.HasPrefix(vv, "?") {
			return nil, fmt.Errorf("Can't have variables (%s) in these things", vv)
		}
		si := ki.String
		if si != nil {
			i, have := si[vv]
			if have {
				ids.AddAll(i.Ids)
				next = append(next, i)
			}
		}

	case Map, map[string]interface{}:
		mi := ki.Map
		if mi != nil {
			var mp map[string]interface{}
			switch v.(type) {
			case Map:
				mp = (map[string]interface{})(v.(Map))
			case map[string]interface{}:
				mp = vv.(map[string]interface{})
			}

			morePairs := mapToPairs(ctx, mp)
			morePairs = append(morePairs, rest...)
			more, err := mi.searchPairs(ctx, morePairs)
			if err != nil {
				return nil, err
			}
			ids.AddAll(more)
			next = append(next, mi)
		}

		// mapPairs := mapToPairs(ctx, &vv)
		// mapPairs := mapToPairs(ctx, mp)
		// rest = append(mapPairs, rest...)

		// mi := ki.Map
		// if mi != nil {
		// 	morePairs := *(mapToPairs(ctx, &vv))
		// 	morePairs = append(morePairs, rest...)
		// 	more, err := mi.searchPairs(ctx, &morePairs)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	ids.AddAll(&more)
		// 	next = append(next, mi)
		// }
	case []interface{}:
		// See mod() above in the same case.
		morePairs := make([]piPair, 0, len(vv))
		sorted, err := SortValues(vv)
		if err != nil {
			return nil, err
		}
		for _, x := range sorted {
			xPair := piPair{k, x}
			morePairs = append(morePairs, xPair)
		}
		rest = append(morePairs, rest...)
	default:
		return nil, fmt.Errorf("can't handle (searchPairs) %v (%T)", v, v)
	}

	for _, ind := range next {
		more, err := ind.searchPairs(ctx, rest)
		if err != nil {
			return nil, err
		}
		ids.AddAll(more)
	}

	return ids, nil
}

// SearchPatternsMap searchs the index for patterns that match the given fact (or event).
func (index *PatternIndex) SearchPatternsMap(ctx *Context, fact map[string]interface{}) (StringSet, error) {
	return index.searchPairs(ctx, mapToPairs(ctx, fact))
}

// AddPatternJSON adds the given pattern (as a map) to the index.
func (index *PatternIndex) AddPatternMap(ctx *Context, m map[string]interface{}, id string) error {
	return index.add(ctx, mapToPairs(ctx, m), id)
}

// RemPatternMap removes the given pattern from the index.
func (index *PatternIndex) RemPatternMap(ctx *Context, m map[string]interface{}, id string) error {
	return index.rem(ctx, mapToPairs(ctx, m), id)
}

// Show prints the index to stdout in a readable way.
func (index *PatternIndex) Show() {
	bs, err := json.MarshalIndent(index, "  ", "  ")
	if err != nil {
		// Can't ever happen?
		fmt.Printf("JSON marshal error: %v\n", err)
	} else {
		fmt.Printf("%s\n", bs)
	}
}
