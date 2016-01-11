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

// Pattern matching

// See "Matching" in 'doc/Manual.md'.

package core

import (
	"fmt"
	"strings"
)

// AllowPropertyVariables enables the experimental support for a
// property variable in a pattern that contains only one property.
var AllowPropertyVariables = true // Parameter

// CheckForBadPropertyVariables runs a test to verify that a pattern
// does not contain a property variable along with other properties.
//
// This check might not be necessary because the other code will
// report an error if a bad property variable is actually enountered
// during matching.  The interesting twist is that if a match fails
// before encountering the bad property variable, then that code will
// not report the problem.  In order to report the problem always,
// turn on this switch.  Performance will suffer, but any bad property
// variable will at least be caught.
var CheckForBadPropertyVariables = true // Parameter

func checkForBadPropertyVariables(pattern map[string]interface{}) error {
	if !CheckForBadPropertyVariables {
		return nil
	}
	if len(pattern) <= 1 {
		return nil
	}
	for k, _ := range pattern {
		if IsVariable(k) {
			return fmt.Errorf("can't have a variable as a key (%s) with other keys", k)
		}
	}
	return nil
}

// Bindings is a map from variables (strings starting with a '?') to
// their values.
type Bindings map[string]interface{}

// Clone returns a cloned (shallow) Bindings.
func (bs *Bindings) Clone() Bindings {
	var copy = make(Bindings)
	for k0, v0 := range *bs {
		copy[k0] = v0
	}
	return copy
}

// IsVariable reports if the string represents a pattern variable.
//
// All pattern variables start with a '?".
func IsVariable(s string) bool {
	return strings.HasPrefix(s, "?")
}

// IsConstant reports if the string represents a constant (and not a
// pattern variable).
func IsConstant(s string) bool {
	return !IsVariable(s)
}

// mapcatMatch attempts to extend the given bindingss 'bss' based on
// pair-wise matching of the pattern to the fact.
func mapcatMatch(ctx *Context, bss []Bindings, pattern map[string]interface{}, fact map[string]interface{}) ([]Bindings, error) {
	//keys := make([]string, len(pattern))
	//i := 0
	//for k, _ := range pattern {
	//	keys[i] = k
	//	i++
	//}
	//sort.Strings(keys)

	//for _, k := range keys {

	if err := checkForBadPropertyVariables(pattern); err != nil {
		return nil, err
	}

	for k, v := range pattern {
		if IsVariable(k) {
			if AllowPropertyVariables {
				if len(pattern) == 1 {
					// Iterate over the fact keys and collect match results.
					gather := make([]Bindings, 0, 0)
					for fk, fv := range fact {
						ext := copyBindingss(bss)

						// Try to match keys.
						ext, err := matchWithBindingss(ctx, ext, k, fk)
						if err != nil {
							return nil, err
						}
						if 0 == len(ext) {
							// Didn't match keys.
							continue
						}
						// Matched keys.  Now check values.
						ext, err = matchWithBindingss(ctx, ext, v, fv)
						if err != nil {
							return nil, err
						}
						if 0 == len(ext) {
							// Didn't match values.
							continue
						}
						gather = append(gather, ext...)
					}
					bss = gather
					// Since we know we have only one key, the outer loop will terminate.
					// Probably should make that termination explicit.
					return bss, nil
				} else {
					return nil, fmt.Errorf("can't have a variable as a key (%s) with other keys", k)
				}
			} else {
				return nil, fmt.Errorf("can't have a variable as a key (%s)", k)
			}
		} else {
			fv, found := fact[k]
			if !found {
				return nil, nil
			}

			acc, err := matchWithBindingss(ctx, bss, v, fv)
			if nil != err {
				return nil, err
			}

			//no match
			if 0 == len(acc) {
				return nil, nil
			}
			bss = acc
		}
	}
	return bss, nil
}

// arraycatMatch attempts to extend the given list of bindingss 'bsss'
// based on element-wise matching.
//
// An array represents a set; therefore, this function can backtrack,
// which can be scary.
func arraycatMatch(ctx *Context, bsss [][]Bindings, pattern interface{}, fxas []map[int]interface{}) ([][]Bindings, []map[int]interface{}, error) {
	var nbsss [][]Bindings
	var nfxas []map[int]interface{}
	for i, bss := range bsss {
		m := fxas[i]
		for j, fact := range m {
			acc, err := matchWithBindingss(ctx, copyBindingss(bss), pattern, fact)
			if nil != err {
				return nil, nil, err
			}
			if 0 != len(acc) {
				nbsss = append(nbsss, acc)
				copy := copyMap(m)
				delete(copy, j)
				nfxas = append(nfxas, copy)
			}
		}
	}
	return nbsss, nfxas, nil
}

func copyMap(source map[int]interface{}) map[int]interface{} {
	target := make(map[int]interface{})
	for p, v := range source {
		target[p] = v
	}
	return target
}

// matchWithBindings attempts to extend the given bindingss 'bss' from
// matches of the fact against the pattern.
//
// Ths function mostly just calls 'Match()'.
func matchWithBindingss(ctx *Context, bss []Bindings, pattern interface{}, fact interface{}) ([]Bindings, error) {
	acc := make([]Bindings, 0, len(bss))
	for _, bs := range bss {
		matches, err := Match(ctx, pattern, fact, bs)
		if nil != err {
			return nil, err
		}
		if nil != matches {
			acc = append(acc, matches...)
		}
	}

	return acc, nil
}

// getVariable finds the first variable and the other non-variables.
// For now, we look for at most one variable.
//
// ToDo: Improve.
func getVariable(ctx *Context, xs []interface{}) (string, []interface{}, error) {
	var v string
	acc := make([]interface{}, 0, len(xs))
	for _, x := range xs {
		switch x.(type) {
		case string:
			s := x.(string)
			if IsVariable(s) {
				if v == "" {
					v = s
					continue
				}
				if v == s {
					err := fmt.Errorf("repeated variables not supported.")
					Log(UERR|MATCH, ctx, "core.getVariable", "xs", xs, "warning", err)
					return "", nil, err
				}

				err := fmt.Errorf("multiple variables not supported here.")
				Log(UERR|MATCH, ctx, "core.getVariable", "xs", xs, "warning", err)
				return "", nil, err
			}
		}
		acc = append(acc, x)
	}
	return v, acc, nil
}

// Matches attempts to match the given fact with the given pattern.
// Returns an array of 'Bindings'.  Each Bindings is just a map from
// variables to their values.
//
// Note that this function returns multiple (sets of) bindings.  This
// ambiguity is introduced when a pattern contains an array that
// contains a variable.
func Matches(ctx *Context, pattern interface{}, fact interface{}) ([]Bindings, error) {
	Log(INFO|MATCH, ctx, "core.Matches", "pattern", pattern, "facti", logFacti(fact))
	return Match(ctx, pattern, fact, make(Bindings))
}

// Match is a verion of 'Matches' that takes initial bindings.
func Match(ctx *Context, pattern interface{}, fact interface{}, bindings Bindings) ([]Bindings, error) {
	// ToDo: Review for garbage reduction.

	if bindings == nil {
		return nil, nil
	}

	p := pattern
	f := fact
	bs := bindings

	Log(DEBUG|MATCH, ctx, "core.Match", "patterni", logFacti(pattern), "facti", logFacti(fact), "bindings", bindings)

	switch vv := p.(type) {
	case nil:
		fmt.Printf("nil null\n")
		switch f.(type) {
		case nil:
			return []Bindings{bindings}, nil
		default:
			return nil, nil
		}

	case bool:
		switch f.(type) {
		case bool:
			y := f.(bool)
			if vv == y {
				return []Bindings{bindings}, nil
			}
			return nil, nil
		default:
			return nil, nil
		}

	case float64:
		switch f.(type) {
		case float64:
			y := f.(float64)
			if vv == y {
				return []Bindings{bindings}, nil
			} else {
				return nil, nil
			}
		default:
			return nil, nil
		}

	case string:
		if IsConstant(vv) {
			switch f.(type) {
			case string:
				fs := f.(string)
				if vv == fs {
					return []Bindings{bindings}, nil
				} else {
					return nil, nil
				}
			default:
				return nil, nil
			}
		} else { // IsVariable
			binding, found := bs[vv]
			if found {
				// check whether new binding is the same as existing
				return Match(ctx, binding, fact, bindings)
			} else {
				// add new binding
				bs[vv] = fact
				return []Bindings{bs}, nil
			}
		}

	case Map, map[string]interface{}:
		m, ok := p.(map[string]interface{})
		if !ok {
			m = map[string]interface{}(p.(Map))
		}
		switch f.(type) {
		case Map, map[string]interface{}:
			fm, ok := f.(map[string]interface{})
			if !ok {
				fm = map[string]interface{}(f.(Map))
			}
			if 0 == len(m) {
				return nil, nil
			}
			return mapcatMatch(ctx, []Bindings{bs}, m, fm)
		default:
			return nil, nil
		}

	case []interface{}:
		//separate variable and constants
		v, xs, err := getVariable(ctx, vv)
		if nil != err {
			return nil, err
		}
		switch f.(type) {
		case []interface{}:

			// index fact array, separate array/map from the string/float fact
			fa := f.([]interface{})
			fxs := make(map[interface{}]bool)
			fxa := make(map[int]interface{})
			for i, y := range fa {
				switch y.(type) {
				case float64, string, bool, nil:
					fxs[y] = true
				default:
					fxa[i] = y
				}
			}

			bsss := [][]Bindings{[]Bindings{bindings}}
			fxas := []map[int]interface{}{fxa}

			// iterate pattern values and match with fact values
			for _, x := range xs {
				switch x.(type) {
				case float64, string, bool, nil:
					_, found := fxs[x]
					if found {
						delete(fxs, x)
					} else {
						return nil, nil
					}
				default:
					if 0 == len(fxa) {
						return nil, nil
					} else {
						bsss, fxas, err = arraycatMatch(ctx, bsss, x, fxas)
						if nil != err {
							return nil, err
						}
						if nil == bsss {
							return nil, nil
						}
					}
				}
			}

			// merge left-over facts
			for _, fxa := range fxas {
				i := len(fa)
				for fact, _ := range fxs {
					fxa[i] = fact
					i++
				}
			}

			// bind pattern variable and match again
			if "" == v {
				return combine(bsss), nil
			} else {
				bsss, fxas, err = arraycatMatch(ctx, bsss, v, fxas)
				if nil != err {
					return nil, err
				}
				return combine(bsss), nil
			}

		default:
			return nil, nil
		}

	default:
		var slice bool
		if p, slice = ISlice(p); slice {
			return Match(ctx, p, fact, bindings)
		}
		return nil, fmt.Errorf("unknown pattern type %T.", p)
	}
}

func combine(bsss [][]Bindings) []Bindings {
	switch len(bsss) {
	case 0:
		return nil
	case 1:
		return bsss[0]
	default:
		var nbss []Bindings
		for _, bss := range bsss {
			nbss = append(nbss, bss...)
		}
		return nbss
	}
}

func copyBindingss(bss []Bindings) []Bindings {
	acc := make([]Bindings, 0, len(bss))
	for _, bs := range bss {
		acc = append(acc, bs.Clone())
	}

	return acc
}
