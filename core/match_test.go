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

// Extract the output as Markdown to build documentation.

package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"testing"
)

func TestBind(t *testing.T) {
	p := fromJSON(`{"a":{"b":["?x"],"c":4}}`)
	bs := make(Bindings)
	bs["?x"] = "B"
	got := bs.Bind(nil, p)
	expected := fromJSON(`{"a":{"b":["B"],"c":4}}`)
	if !reflect.DeepEqual(got, expected) {
		t.Error(fmt.Errorf("TestBind: Not equal: %v and %v\n", got, expected))
	}
}

func TestMatchArrayBadVars(t *testing.T) {
	p := fromJSON(`{"a":["?x","?y"]}`)
	f := fromJSON(`{"a":[1]}`)
	Matches(nil, p, f)
}

func TestMatchBadVarAsKey(t *testing.T) {
	if AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"?a":1}`)
	f := fromJSON(`{"a":1}`)
	_, err := Matches(nil, p, f)
	if err == nil {
		t.Fail()
	}
}

func TestMatchVarAsKey(t *testing.T) {
	if !AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"?a":1}`)
	f := fromJSON(`{"a":1, "b":2}`)
	bss, err := Matches(nil, p, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(bss) != 1 {
		t.Fail()
	}
	got, have := bss[0]["?a"]
	if !have || got != "a" {
		t.Fail()
	}
}

func TestMatchVarAsKeyMultiple(t *testing.T) {
	if !AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"?a":1}`)
	f := fromJSON(`{"a":1, "b":2, "c":1}`)
	bss, err := Matches(nil, p, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(bss) != 2 {
		t.Logf("bss %#v", bss)
		t.Fail()
	}
	got, have := bss[0]["?a"]
	if !have || (got != "a" && got != "c") {
		t.Logf("bss %#v", bss)
		t.Fail()
	}
}

func TestMatchVarAsKeyMultipleWithVarVal(t *testing.T) {
	if !AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"?x":"?y"}`)
	f := fromJSON(`{"a":"1", "b":"2", "c":"1"}`)
	bss, err := Matches(nil, p, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(bss) != 3 {
		t.Logf("bss %#v", bss)
		t.Fail()
	}
	y, have := bss[0]["?y"]
	if !have || (y != "1" && y != "2") {
		t.Logf("bss %#v %#v", bss, y)
		t.Fail()
	}
}

func TestMatchBadVarInKeys(t *testing.T) {
	// Can never tolerate a variable property along with another
	// property.  We try several patterns.  Since map ranging is
	// Go has a randomized order, we need multiple permutations to
	// catch a bug consistently.

	// Detecting an illegal bad variable in keys is hard to do
	// efficiently.  Example: A match could fail without ever
	// getting to the bad variable key.  In that case, we might
	// never report the problem.

	ps := []string{`{"?a":1, "b":2}`,
		`{"a":1, "?b":2}`,
		`{"b":1, "?a":2}`,
		`{"a":1, "b":2, "?c":3}`,
		`{"a":1, "?b":2, "c":3}`,
	}
	for _, pat := range ps {
		p := fromJSON(pat)
		f := fromJSON(`{"a":1}`)
		_, err := Matches(nil, p, f)
		if err == nil {
			t.Logf("failed on %s", pat)
			t.Fail()
		}
	}
}

func TestMatchBadVarAsDeepKey(t *testing.T) {
	if AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"b": {"?a":1}}`)
	f := fromJSON(`{"b": {"a":1}}`)
	_, err := Matches(nil, p, f)
	if err == nil {
		t.Fail()
	}
}

func TestMatchVarAsDeepKey(t *testing.T) {
	if !AllowPropertyVariables {
		t.Skip()
	}
	p := fromJSON(`{"b": {"?a":1}}`)
	f := fromJSON(`{"b": {"a":1, "c":2}}`)
	bss, err := Matches(nil, p, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(bss) != 1 {
		t.Logf("bss %#v", bss)
		t.Fail()
	}
	got, have := bss[0]["?a"]
	if !have || got != "a" {
		t.Fail()
	}
}

func TestMatchBadVarInDeepKeys(t *testing.T) {
	if AllowPropertyVariables {
		t.Skip()
	}
	// Multiple keys.
	p := fromJSON(`{"b": {"?a":1, "c":2}}`)
	f := fromJSON(`{"b": {"a":1}}`)
	_, err := Matches(nil, p, f)
	if err == nil {
		t.Fail()
	}
}

func TestMatchNilBindings(t *testing.T) {
	bss, err := Match(nil, nil, nil, nil)
	if bss != nil || err != nil {
		t.Fail()
	}
}

func TestMatchWeirdPattern(t *testing.T) {
	f := fromJSON(`{"b": {"a":1}}`)
	p := struct{}{}
	_, err := Matches(nil, p, f)
	if err == nil {
		t.Fail()
	}
}

func TestMatchEmptyMap(t *testing.T) {
	f := fromJSON(`{"b": {"a":1}}`)
	p := map[string]interface{}{}
	m, err := Matches(nil, p, f)
	if m != nil || err != nil {
		t.Fail()
	}
}

func TestMatchNullValue(t *testing.T) {
	f := fromJSON(`{"b":null, "a":1}`)
	p := fromJSON(`{"b":null, "a":"?x"}`)
	m, err := Matches(nil, p, f)
	fmt.Printf("matched %v\n", m)
	if m == nil || len(m) == 0 || err != nil {
		t.Fail()
	}
}

func TestMatchTypeConflict(t *testing.T) {
	p := fromJSON(`{"a": 1}`)
	f := fromJSON(`{"a": "A"}`)
	bss, err := Matches(nil, p, f)
	if err != nil {
		t.Error("num versus string (err)")
	}
	if 0 < len(bss) {
		t.Error("num versus string")
	}

	p = fromJSON(`{"a": "A"}`)
	f = fromJSON(`{"a": 1}`)
	bss, err = Matches(nil, p, f)
	if err != nil {
		t.Error("string versus num (err)")
	}
	if 0 < len(bss) {
		t.Error("string versus num")
	}

	p = fromJSON(`{"a": true}`)
	f = fromJSON(`{"a": 1}`)
	bss, err = Matches(nil, p, f)
	if err != nil {
		t.Error("bool versus num (err)")
	}
	if 0 < len(bss) {
		t.Error("bool versus num")
	}

	p = fromJSON(`{"a": {"b": "B"}}`)
	f = fromJSON(`{"a": 1}`)
	bss, err = Matches(nil, p, f)
	if err != nil {
		t.Error("map versus num (err)")
	}
	if 0 < len(bss) {
		t.Error("map versus num")
	}
}

// Probably should have made pattern, fact map[string]interface{}s
// instead of strings (JSON).
type MatchTest struct {
	pattern  map[string]interface{}
	fact     map[string]interface{}
	expected []map[string]interface{}
	// Set to true if the match should actually return an error.
	err     bool
	comment string
}

func makeMatchTestFromJSON(ctx *Context, js string) *MatchTest {
	m := fromJSON(js)
	expected := m["expected"].([]interface{}) // Annoying!
	es := make([]map[string]interface{}, 0, len(expected))
	for _, e := range expected {
		es = append(es, e.(map[string]interface{}))
	}

	comment, given := m["comment"]
	var c string
	if given {
		c = comment.(string)
	}

	return &MatchTest{m["pattern"].(map[string]interface{}),
		m["fact"].(map[string]interface{}),
		es,
		false,
		c}
}

// compareResult compares the matching result with expected
func CompareMatchResult(bss []Bindings, expected []map[string]interface{}) bool {
	if len(bss) != len(expected) {
		return false
	}

	m := make(map[int]map[string]interface{})
	for i, got := range bss {
		m[i] = map[string]interface{}(got)
	}

	for _, e := range expected {
		found := false
		for k, v := range m {
			if reflect.DeepEqual(e, v) {
				delete(m, k)
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return 0 == len(m)
}

func runMatchTest(ctx *Context, m *MatchTest, label string, t *testing.T) bool {
	expected, err := json.Marshal(&m.expected)
	if err != nil {
		panic(err)
	}

	bss, err := Matches(ctx, m.pattern, m.fact)
	if err != nil {
		return m.err
	}

	if CompareMatchResult(bss, m.expected) {
		fmt.Printf("matchTest passed %s | %s | %s | %s | %s |\n",
			label, toJSON(m.pattern), toJSON(m.fact), expected, m.comment)
		return true
	} else {
		t.Errorf("matchTest failed %s pattern: %s fact: %s got: %v expected: %s\n",
			label, m.pattern, m.fact, bss, m.expected)
		return false
	}
}

func TestMatchMany(t *testing.T) {
	ctx := NewContext("TestMatchMany")

	dir := "matchtest"
	files, _ := ioutil.ReadDir(dir)
	for _, f := range files {
		filename := dir + "/" + f.Name()
		if !strings.HasSuffix(filename, ".js") {
			continue
		}
		bs, err := ioutil.ReadFile(filename)
		if err != nil {
			panic(err)
		}
		runMatchTest(ctx, makeMatchTestFromJSON(ctx, string(bs)), filename, t)
	}
}

func benchmarkMatch(b *testing.B, name string, patternStr string, factStr string) {
	ctx := BenchContext(name)
	pattern := fromJSON(patternStr)
	fact := fromJSON(factStr)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Matches(ctx, pattern, fact)
	}
}

func BenchmarkMatchSimplest(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchSimplest", `{"a":"?x"}`, `{"a":"A"}`)
}

func BenchmarkMatchArray1(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray1", `{"a":["?x"]}`, `{"a":[1]}`)
}

func BenchmarkMatchArray2(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray2", `{"a":["?x"]}`, `{"a":[1,2]}`)
}

func BenchmarkMatchArray4(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray4", `{"a":["?x"]}`, `{"a":[1,2,3,4]}`)
}

func BenchmarkMatchArray8(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray8", `{"a":["?x"]}`, `{"a":[1,2,3,4,5,6,7,8]}`)
}

func BenchmarkMatchArray16(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray16", `{"a":["?x"]}`, `{"a":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]}`)
}

func BenchmarkMatchArray4x4(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray4x4", `{"a":["?x"],"b":["?y"]}`, `{"a":[1,2,3,4],"b":[1,2,3,4]}`)
}

func BenchmarkMatchArray4x4Same(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchArray4x4Same", `{"a":["?x"],"b":["?x"]}`, `{"a":[1,2,3,4],"b":[1,2,3,4]}`)
}

func BenchmarkMatchMapSimple(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchMapSimple", `{"a":"?x", "b":"?y", "c":"?z"}`, `{"a":1, "b":2, "c":3}`)
}

func BenchmarkMatchMapDeeper(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchMapDeeper", `{"a":{"b":{"c":"?x"}}}`, `{"a":{"b":{"c":3}}}`)
}

func BenchmarkMatchMapDeeperer(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchMapDeeperer", `{"a":{"b":{"c":{"d":{"e":"?x"}}}}}`, `{"a":{"b":{"c":{"d":{"e":5}}}}}`)
}

func BenchmarkMatchMapWide1(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchMapWide1", `{"g":"?g"}`, `{"a":1, "b":2, "c":3, "d":4, "e":5, "f":6, "g": 7}`)
}

func BenchmarkMatchMapWide2(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchMapWide2", `{"a":"?a"}`, `{"a":1, "b":2, "c":3, "d":4, "e":5, "f":6, "g": 7}`)
}

func BenchmarkMatchStandard(b *testing.B) {
	benchmarkMatch(b, "BenchmarkMatchStandard",
		`{"a":"?a","b":{"c":"?c"}, "d":"D"}`,
		`{"a":  1 ,"b":{"c":  2 }, "d":"D"}`)
}
