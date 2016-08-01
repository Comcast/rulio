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
	// "sort"
	"testing"
)

// SearchPatternsMap searchs the index for patterns that match the given fact (or event) (in JSON).
func (index *PatternIndex) SearchPatternsJSON(ctx *Context, js []byte) (StringSet, error) {
	Log(DEBUG, ctx, "PatternIndex.SearchPatternsJSON", "js", string(js))
	fact, err := ParseJSON(ctx, js)
	if err == nil {
		return index.SearchPatternsMap(ctx, fact)
	}
	return nil, err
}

// AddPatternJSON adds the given pattern (as JSON) to the index.
func (index *PatternIndex) AddPatternJSON(ctx *Context, js []byte, id string) error {
	pattern, err := ParseJSON(ctx, js)
	if err != nil {
		return err
	}
	return index.add(ctx, mapToPairs(ctx, pattern), id)
}

// checkSearch queries a PatternIndex and checks the results.
//
// First parse 'js' as JSON representation of a fact/event.  The query
// the given PatternIndex.  Compare the results (pattern/rule ids)
// with what was 'expect'ed.  If we didn't get what we expected, call
// t.Fail() sand log why.
func checkSearch(t *testing.T, i *PatternIndex, js string, expect []string) {
	found, err := i.SearchPatternsJSON(nil, []byte(js))
	if err != nil {
		t.Errorf("Error %v on %v\n", err, js)
		return
	}
	left, right := found.Difference(NewStringSet(expect))
	if len(left) == 0 && len(right) == 0 {
	} else {
		t.Errorf("%s surprised by %v and missed %v\n", js, left, right)
	}
}

func TestPatternStoreBasic(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1"})

	i.AddPatternJSON(nil, []byte(`{"a":"2"}`), "p2")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1"})
	checkSearch(t, i, `{"a":"2"}`, []string{"p2"})

	i.AddPatternJSON(nil, []byte(`{"a":"?x"}`), "p3")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1", "p3"})
	checkSearch(t, i, `{"a":"2"}`, []string{"p2", "p3"})
	checkSearch(t, i, `{"a":{"b":"1"}}`, []string{"p3"})

	i.AddPatternJSON(nil, []byte(`{"a":"3","b":"4"}`), "p4")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1", "p3"})
	checkSearch(t, i, `{"a":"2"}`, []string{"p2", "p3"})
	checkSearch(t, i, `{"a":"3"}`, []string{"p3"})
	checkSearch(t, i, `{"a":"3","b":"4"}`, []string{"p3", "p4"})
	checkSearch(t, i, `{"a":["3","4"]}`, []string{"p3"})

	i.AddPatternJSON(nil, []byte(`{"c":["?x"]}`), "p5")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1", "p3"})
	checkSearch(t, i, `{"a":"2"}`, []string{"p2", "p3"})
	checkSearch(t, i, `{"a":"3"}`, []string{"p3"})
	checkSearch(t, i, `{"a":"3","b":"4"}`, []string{"p3", "p4"})
	checkSearch(t, i, `{"a":["3","4"]}`, []string{"p3"})
	checkSearch(t, i, `{"c":["3","4"]}`, []string{"p5"})

	// i.Show()
}

func TestPatternStoreSearchOrdering(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1","b":"2","c":"3"}`), "p1")
	i.AddPatternJSON(nil, []byte(`{"a":"1","b":"2"}`), "p1")
	i.AddPatternJSON(nil, []byte(`{"b":"2"}`), "p1")
	checkSearch(t, i, `{"a":"1", "b":"2", "c":"3"}`, []string{"p1"})
	checkSearch(t, i, `{"b":"2", "c":"3", "a":"1"}`, []string{"p1"})
	checkSearch(t, i, `{"c":"3", "a":"1", "b":"2"}`, []string{"p1"})
	checkSearch(t, i, `{"a":"1", "c":"3", "b":"2"}`, []string{"p1"})
}

func TestPatternStoreAddBadVarProp(t *testing.T) {
	if AllowPropertyVariables {
		t.Skip()
	}
	i := NewPatternIndex()
	err := i.AddPatternJSON(nil, []byte(`{"?":"1"}`), "p1")
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreAddOkayVarProp(t *testing.T) {
	if !AllowPropertyVariables {
		t.Skip()
	}
	i := NewPatternIndex()
	err := i.AddPatternJSON(nil, []byte(`{"?":"1"}`), "p1")
	if err != nil {
		t.Fail()
	}
}

func TestPatternStoreAddBadJSON(t *testing.T) {
	i := NewPatternIndex()
	err := i.AddPatternJSON(nil, []byte(`{bad:json}`), "p1")
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchBadJSON(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{bad:json}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchBadKey(t *testing.T) {
	if AllowPropertyVariables {
		t.Skip()
	}
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{"?bad":1}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchBadValue(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{"a":"?bad"}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchMapRestBadValue(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":{"b":"1"}}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{"a":{"b":"?bad"}}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchMapBadValue(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":1, "b": {"c":3}}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{"a":1, "b":{"c":"?bad"}}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSearchBadArray(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":1, "b": 2}`), "p1")
	_, err := i.SearchPatternsJSON(nil, []byte(`{"a":[1,false], "b":2}`))
	if err == nil {
		t.Fail()
	}
}

func TestPatternStoreSimpleBool(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":false}`), "p1")
	checkSearch(t, i, `{"a":false}`, []string{"p1"})
	i.AddPatternJSON(nil, []byte(`{"a":true}`), "p2")
	checkSearch(t, i, `{"a":true}`, []string{"p2"})
}

func TestPatternStoreSimpleNum(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":1}`), "p1")
	checkSearch(t, i, `{"a":1}`, []string{"p1"})
}

func TestPatternStoreSimpleString(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"A"}`), "p1")
	checkSearch(t, i, `{"a":"A"}`, []string{"p1"})
}

func TestPatternStoreSimpleMap(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":{"b":"B"}}`), "p1")
	checkSearch(t, i, `{"a":{"b":"B"}}`, []string{"p1"})
}

func TestPatternStoreSimpleArray(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":["A"]}`), "p1")
	checkSearch(t, i, `{"a":["A"]}`, []string{"p1"})
}

func TestPatternStoreArrayBad(t *testing.T) {
	i := NewPatternIndex()
	if nil == i.AddPatternJSON(nil, []byte(`{"a":[42, "A"]}`), "p1") {
		t.Fail()
	}
}

func TestPatternStoreArrayEmpty(t *testing.T) {
	i := NewPatternIndex()
	if nil != i.AddPatternJSON(nil, []byte(`{"a":[]}`), "p1") {
		t.Fail()
	}
}

func TestPatternStoreArrayHomogeneous(t *testing.T) {
	xs := []interface{}{1, 2, 3}
	if !IsSortable(xs) {
		t.Fail()
	}
}

func TestPatternStoreArrayHomogeneousBad(t *testing.T) {
	xs := []interface{}{1, 2, "three"}
	if IsSortable(xs) {
		t.Fail()
	}
}

func TestPatternStoreDeepBool(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":{"b": false}}`), "p1")
	checkSearch(t, i, `{"a":{"b":false}}`, []string{"p1"})
	checkSearch(t, i, `{"a":{"b":true}}`, []string{})
}

func TestPatternStoreOkayVarProp(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"?p":"1"}`), "p1")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1"})
	checkSearch(t, i, `{"a":"1","b":"2"}`, []string{"p1"})
	checkSearch(t, i, `{"a":"3","b":"2"}`, []string{})
}

func TestPatternStoreArrayBool(t *testing.T) {
	i := NewPatternIndex()
	// Stupid, but get us 100% coverage of picast()!
	if nil != i.AddPatternJSON(nil, []byte(`{"a":[true,false]}`), "p1") {
		t.Fail()
	}
}

func TestPatternStoreAddMapInts(t *testing.T) {
	i := NewPatternIndex()
	m := make(map[string]interface{})
	m["a"] = 1 // An int!
	if nil != i.AddPatternMap(nil, m, "p1") {
		t.Fail()
	}
}

func TestPatternStoreAddMapArrayInts(t *testing.T) {
	i := NewPatternIndex()
	m := make(map[string]interface{})
	m["a"] = []int{3, 2, 1}
	// We expect this call to fail because we don't current support non-interface slices!
	if nil == i.AddPatternMap(nil, m, "p1") {
		t.Fail()
	}
}

func TestPatternStoreSearchMapArrayInts(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	m := make(map[string]interface{})
	m["a"] = []int{3, 2, 1}
	// We expect this call to fail because we don't current support non-interface slices!
	if _, err := i.SearchPatternsMap(nil, m); err == nil {
		t.Fail()
	}
}

func TestPatternStoreAddMapArrayInterfaces(t *testing.T) {
	i := NewPatternIndex()
	m := make(map[string]interface{})
	m["a"] = []interface{}{3, 2, 1}
	if nil != i.AddPatternMap(nil, m, "p1") {
		t.Fail()
	}
}

func TestPatternStoreAddMapArrayHetero(t *testing.T) {
	i := NewPatternIndex()
	m := make(map[string]interface{})
	m["a"] = []interface{}{3, "two", 1}
	if err := i.AddPatternMap(nil, m, "p1"); err == nil {
		t.Fatal("should have protested")
	}
}

func TestPatternStoreAddMapArrayComplexTwo(t *testing.T) {
	i := NewPatternIndex()
	if err := i.AddPatternJSON(nil, []byte(`{"a":[{"b":"two"},{"c":"three"}]}`), "p2"); err == nil {
		t.Fatal("should have protested")
	}
}

func TestPatternStoreAddMapArrayComplexSingleton(t *testing.T) {
	// A complex array that just contains a single (complex) element should be okay.
	i := NewPatternIndex()
	if err := i.AddPatternJSON(nil, []byte(`{"b":[{"a":"one"}]}`), "p2"); err != nil {
		t.Fatal(err)
	}
	checkSearch(t, i, `{"b":[{"a":"one"}]}`, []string{"p2"})
}

func TestPatternStoreArrayVarious(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"b":[1,2]}`), "p2")
	checkSearch(t, i, `{"b":[1]}`, []string{})
	checkSearch(t, i, `{"b":[1,2]}`, []string{"p2"})
	checkSearch(t, i, `{"b":[2]}`, []string{})
}

func TestPatternStoreArrayOrder(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"b":[2,1]}`), "p2")
	checkSearch(t, i, `{"b":[1]}`, []string{})
	checkSearch(t, i, `{"b":[2]}`, []string{})
	checkSearch(t, i, `{"b":[2,1]}`, []string{"p2"})
	checkSearch(t, i, `{"b":[1,2]}`, []string{"p2"})
}

func TestPatternStoreNoConstructor(t *testing.T) {
	i := &PatternIndex{}
	i.AddPatternJSON(nil, []byte(`{}`), "p1")
}

func TestPatternStoreRemBasic(t *testing.T) {
	i := NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	checkSearch(t, i, `{"a":"1"}`, []string{"p1"})

	m := map[string]interface{}{"a": "1"}
	i.RemPatternMap(nil, m, "p1")
	checkSearch(t, i, `{"a":"1"}`, []string{})

	i = NewPatternIndex()
	i.AddPatternJSON(nil, []byte(`{"a":"1"}`), "p1")
	i.AddPatternJSON(nil, []byte(`{"a":"1","b":"2"}`), "p2")
	checkSearch(t, i, `{"a":"1","b":"2"}`, []string{"p1", "p2"})

	m = map[string]interface{}{"a": "1"}
	i.RemPatternMap(nil, m, "p1")
	checkSearch(t, i, `{"a":"1","b":"2"}`, []string{"p2"})
}

func TestPatternIndexNilShow(t *testing.T) {
	var i *PatternIndex
	i.Show()
}

func piExample(ctx *Context, i *PatternIndex, js string, id string) {
	fmt.Printf("Adding '%s' with id %s\n\n", js, id)
	err := i.AddPatternJSON(ctx, []byte(js), id)
	if err != nil {
		panic(err)
	}
	i.Show()
}

// This example sometimes breaks due to -- I think -- Go's map range
// randomization.
//
// func ExamplePatternIndex() {
// 	ctx := TestContext("ExamplePatternIndexDoc")
// 	i := NewPatternIndex()
// 	i.AddPatternJSON(ctx, []byte(`{"a":{"b":"B"}}`), "p1")
// 	i.AddPatternJSON(ctx, []byte(`{"a":{"c":"C"}}`), "p2")
// 	i.AddPatternJSON(ctx, []byte(`{"a":"?X"}`), "p3")
// 	i.Show()
// 	found, _ := i.SearchPatternsJSON(ctx, []byte(`{"a":{"b":"B"}}`))
// 	ids := found.Array()
// 	sort.Strings(*ids)
// 	fmt.Println(*ids)
// 	// Output:
// 	// {
// 	//     "String": {
// 	//       "a": {
// 	//         "String": null,
// 	//         "Var": {
// 	//           "String": null,
// 	//           "Var": null,
// 	//           "Map": null,
// 	//           "IDs": {
// 	//             "p3": {}
// 	//           }
// 	//         },
// 	//         "Map": {
// 	//           "String": {
// 	//             "b": {
// 	//               "String": {
// 	//                 "S_B": {
// 	//                   "String": null,
// 	//                   "Var": null,
// 	//                   "Map": null,
// 	//                   "IDs": {
// 	//                     "p1": {}
// 	//                   }
// 	//                 }
// 	//               },
// 	//               "Var": null,
// 	//               "Map": null,
// 	//               "IDs": {}
// 	//             },
// 	//             "c": {
// 	//               "String": {
// 	//                 "S_C": {
// 	//                   "String": null,
// 	//                   "Var": null,
// 	//                   "Map": null,
// 	//                   "IDs": {
// 	//                     "p2": {}
// 	//                   }
// 	//                 }
// 	//               },
// 	//               "Var": null,
// 	//               "Map": null,
// 	//               "IDs": {}
// 	//             }
// 	//           },
// 	//           "Var": null,
// 	//           "Map": null,
// 	//           "IDs": {}
// 	//         },
// 	//         "IDs": {}
// 	//       }
// 	//     },
// 	//     "Var": null,
// 	//     "Map": null,
// 	//     "IDs": {}
// 	//   }
// 	// [p1 p3]
// }

// TestPatternIndexDoc will generate some documentation for examples.
//
// See it: 'go test -test.run=PatternIndexDoc'
func TestPatternIndexDoc(t *testing.T) {
	ctx := TestContext("ExamplePatternIndexDoc")

	i := NewPatternIndex()

	piExample(ctx, i, `{"a":"A"}`, "paA")

	piExample(ctx, i, `{"b":"B"}`, "pbB")

	piExample(ctx, i, `{"a":"A", "b": "B"}`, "paAbB")

	piExample(ctx, i, `{"b":"B","c":"C"}`, "pbBcC")

	piExample(ctx, i, `{"a":{"x":"X"}}`, "paxX")

	piExample(ctx, i, `{"a":{"x":"X","y":"Y"}}`, "paxXyY")

	piExample(ctx, i, `{"a":{"y":"Y"}}`, "payY")

	piExample(ctx, i, `{"a":"?A","b":"B"}`, "pa?bB")

	i = NewPatternIndex()

	piExample(ctx, i, `{"c":{"e":"E"}, "d":"D", "a":{"b":"?B"}}`, "pab?BcdDeE")

	i = NewPatternIndex()

	piExample(ctx, i, `{"c":{"e":"E","d":"D"}, "a":{"b":"?B"}}`, "pab?BcdDeE")
}

func BenchmarkTypeCode(b *testing.B) {
	y := []interface{}{}
	xs := []interface{}{"foo", int(3), float64(4.0), struct{}{}, y}
	for i := 0; i < b.N; i++ {
		_ = typeCode(xs[i%len(xs)])
	}
}

func BenchmarkIsSortable(b *testing.B) {
	x := struct{}{}
	xs := []interface{}{
		[]interface{}{1, 2, 3, 4},
		[]interface{}{"a", "b", "c"},
		[]interface{}{"a", 2, "c", 4},
		[]interface{}{"a", 2, "c", 4},
		[]interface{}{x},
		[]interface{}{x, x},
		[]interface{}{x, x, x},
		[]interface{}{1, 2, 3, 4, x},
	}
	for i := 0; i < b.N; i++ {
		_ = IsSortable(xs[i%len(xs)].([]interface{}))
	}
}

func BenchmarkPatternIndexAdd(b *testing.B) {

	numPatterns := 100

	ctx := BenchContext("BenchmarkPatternIndex")

	// i := NewPatternIndex()

	patterns := make([]map[string]interface{}, 0, 0)
	count := 0
	for _, pattern := range genFacts(ctx, numPatterns, 3, 3, 2, true) {
		count++
		// id := fmt.Sprintf("p%d", count)
		// js, err := json.Marshal(pattern)
		// if err != nil {
		// 	b.Logf("error %v", err)
		// 	b.Fail()
		// }
		// fmt.Printf("pattern: %s\n", js)
		patterns = append(patterns, pattern)

		// err := i.AddPatternMap(ctx, (*map[string]interface{})(pattern), id)
		// if err != nil {
		// 	b.Fatal(err)
		// }
	}

	b.ResetTimer()

	pi := NewPatternIndex()
	for i := 0; i < b.N; i++ {
		pattern := patterns[i%len(patterns)]
		id := fmt.Sprintf("p%d", i)
		err := pi.AddPatternMap(ctx, pattern, id)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPatternIndexSearch(b *testing.B) {

	numPatterns := 1000
	numEvents := 1000

	ctx := BenchContext("BenchmarkPatternIndex")

	pi := NewPatternIndex()
	count := 0
	for _, pattern := range genFacts(ctx, numPatterns, 3, 3, 2, true) {
		count++
		id := fmt.Sprintf("p%d", count)
		err := pi.AddPatternMap(ctx, (map[string]interface{})(pattern), id)
		if err != nil {
			b.Fatal(err)
		}
	}

	events := make([]map[string]interface{}, 0, 0)
	for _, event := range genFacts(ctx, numEvents, 3, 3, 2, false) {
		events = append(events, event)
	}

	b.ResetTimer()

	found := 0
	for i := 0; i < b.N; i++ {
		event := events[i%len(events)]
		got, err := pi.SearchPatternsMap(ctx, event)
		if err != nil {
			b.Fatal(err)
		}
		found += len(got)
	}
	// b.Logf("Total found: %d", found)
}
