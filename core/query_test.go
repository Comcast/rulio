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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type SearchTest struct {
	bsss [][]Bindings
}

func NewSearchTest(bsss string) *SearchTest {
	qt := SearchTest{}

	if err := json.Unmarshal([]byte(bsss), &qt.bsss); nil != err {
		panic(err)
	}
	return &qt
}

func (m *Bindings) sameAs(bs Bindings) bool {
	missed := len(*m)
	extra := len(bs)
	for k, x := range bs {
		y, have := (*m)[k]
		if !have {
			return false
		}
		if x != y {
			return false
		}
		missed--
		extra--
	}
	return missed == 0 && extra == 0
}

func sameBindingss(x []Bindings, y []Bindings) bool {
	if len(x) != len(y) {
		return false
	}

	yc := make([]Bindings, len(y))
	copy(yc, y)
	y = yc
XS:
	for _, bs := range x {
		for i, bs1 := range y {
			if bs.sameAs(bs1) {
				y = append(y[:i], y[i+1:]...)
				continue XS
			}
		}
		return false
	}
	return true
}

func (qt *SearchTest) got(srs SearchResults) bool {
	if len(qt.bsss) != len(srs.Found) {
		return false
	}
LOOP:
	for _, bss := range qt.bsss {
		for _, sr := range srs.Found {
			bss1 := sr.Bindingss
			if sameBindingss(bss, bss1) {
				continue LOOP
			}
		}
		return false
	}
	return true
}

func doSearchTest(t *testing.T, facts []string, pattern string, bsss string) {
	ctx := NewContext("TestSystem")
	loc, err := NewLocation(ctx, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = loc.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	for _, fact := range facts {
		if _, err := loc.AddFact(ctx, "", mapJS(fact)); nil != err {
			t.Error(err)
		}
	}

	srs, err := loc.SearchFacts(ctx, mapJS(pattern), true)

	if nil != err {
		t.Error(err)
	}

	if !NewSearchTest(bsss).got(*srs) {
		js, _ := json.MarshalIndent(&srs.Found, "  ", "  ")
		t.Logf("Got %s\n", js)
		t.Fail()
	}
}

func TestSearchSimple(t *testing.T) {
	doSearchTest(t,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`},
		`{"likes":"?what"}`,
		`[[{"?what":"tacos"}],[{"?what":"chips"}]]`)
}

func TestSearch1(t *testing.T) {
	doSearchTest(t,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"likes":"?likes"}`,
		`[[{"?likes":"tacos"}],[{"?likes":"chips"}]]`)
}

func ParseQueryFromJSON(ctx *Context, s string) (Query, error) {
	var x map[string]interface{}
	err := json.Unmarshal([]byte(s), &x)
	if err != nil {
		return nil, err
	}
	return ParseQuery(ctx, x)
}

func doQueryTest(t *testing.T, facts []string, query string, bss string) error {
	ctx := NewContext("TestSystem")
	loc, err := NewLocation(ctx, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = loc.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	for _, fact := range facts {
		if _, err := loc.AddFact(ctx, "", mapJS(fact)); nil != err {
			t.Error(err)
		}
	}

	q, err := ParseQueryFromJSON(ctx, query)
	if nil != err {
		t.Error(err)
	}

	qc := QueryContext{}
	qr, err := q.Exec(ctx, loc, qc, InitialQueryResult(ctx))

	if nil != err {
		return err
	}

	var expected []Bindings
	err = json.Unmarshal([]byte(bss), &expected)
	if nil != err {
		t.Error(err)
	}
	if !sameBindingss(expected, qr.Bss) {
		js, _ := json.MarshalIndent(&qr.Bss, "  ", "  ")
		t.Logf("Got %s\n", js)
		t.Fail()
	}

	return nil
}

func TestQuerySimple(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"pattern": {"likes":"?likes"}}`,
		`[{"?likes":"tacos"},{"?likes":"chips"}]`)
	assert.NoError(t, err)
}

func TestQueryExternal(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"found":[{"bindingss":[{"?likes":"tacos"},{"?likes":"chips"}]}]}`))
			return
		}))
		defer server.Close()
		err := doQueryTest(t,
			nil,
			fmt.Sprintf(`{"pattern": {"likes":"?likes"}, "location": %q}`, server.URL),
			`[{"?likes":"tacos"},{"?likes":"chips"}]`)
		assert.NoError(t, err)
	})

	t.Run("ExternalError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}))
		defer server.Close()
		err := doQueryTest(t,
			nil,
			fmt.Sprintf(`{"pattern": {"likes":"?likes"}, "location": %q}`, server.URL),
			`[{"?likes":"tacos"},{"?likes":"chips"}]`)
		assert.Error(t, err)
		if err != nil {
			assert.Contains(t, err.Error(), "unexpected status code")
		}
	})
}

func TestQueryOr(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"or":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?drinks"}}]}`,
		`[{"?likes":"tacos"},{"?likes":"chips"},{"?drinks":"beer"}]`)
	assert.NoError(t, err)
}

func TestQueryOrShortCircuit(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"shortCircuit": true, "or":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?drinks"}}]}`,
		`[{"?likes":"tacos"},{"?likes":"chips"}]`)
	assert.NoError(t, err)
}

func TestQueryAnd(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?likes"}}]}`,
		`[{"?likes":"rum"}]`)
	assert.NoError(t, err)
}

func TestQueryNot(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"not":{"pattern": {"drinks":"?likes"}}}]}`,
		`[{"?likes":"tacos"}]`)
	assert.NoError(t, err)
}

func TestQueryCode(t *testing.T) {
	err := doQueryTest(t,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"code":"likes.length < 4"}]}`,
		`[{"?likes":"rum"}]`)
	assert.NoError(t, err)
}

func doQueryBenchmark(b *testing.B, facts []string, query string, bss string) {
	ctx := BenchContext("QueryBenchmark")
	loc, err := NewLocation(ctx, "test", nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	if err = loc.Clear(ctx); err != nil {
		b.Fatal(err)
	}

	for _, fact := range facts {
		if _, err := loc.AddFact(ctx, "", mapJS(fact)); nil != err {
			b.Error(err)
		}
	}

	q, err := ParseQueryFromJSON(ctx, query)
	if nil != err {
		b.Error(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		qc := QueryContext{}
		if _, err = q.Exec(ctx, loc, qc, InitialQueryResult(ctx)); err != nil {
			b.Error(err)
		}
		// Correctness should be checked by doQueryTest() from other tests.
	}
}

func BenchmarkQuerySimple(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"pattern": {"likes":"?likes"}}`,
		`[{"?likes":"tacos"},{"?likes":"tacos"}]`)
}

func BenchmarkQueryOr(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"or":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?drinks"}}]}`,
		`[{"?likes":"tacos"},{"?likes":"tacos"},{"?drinks":"beer"}]`)
}

func BenchmarkQueryOrShortCircuit(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"chips"}`, `{"likes":"tacos"}`, `{"drinks":"beer"}`},
		`{"shortCircuit": true, "or":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?drinks"}}]}`,
		`[{"?likes":"tacos"},{"?likes":"chips"}]`)
	}

func BenchmarkQueryAnd(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"pattern": {"drinks":"?likes"}}]}`,
		`[{"?likes":"rum"}]`)
}

func BenchmarkQueryNot(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"not":{"pattern": {"drinks":"?likes"}}}]}`,
		`[{"?likes":"tacos"}]`)
}

func BenchmarkQueryCode(b *testing.B) {
	doQueryBenchmark(b,
		[]string{`{"likes":"rum"}`, `{"likes":"tacos"}`, `{"drinks":"rum"}`},
		`{"and":[{"pattern": {"likes":"?likes"}},{"code":"likes.length < 4"}]}`,
		`[{"?likes":"rum"}]`)
}

func TestLocationsFromMapBad(t *testing.T) {
	m := map[string]interface{}{"location": 42}
	_, err := getLocationsFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestLocationsFromMapGood(t *testing.T) {
	m := map[string]interface{}{"location": "there"}
	there, err := getLocationsFromMap(nil, m)
	if err != nil {
		t.Fatalf("shouldn't have reported error %v", err)
	}
	if len(there) != 1 {
		t.Fatalf("why in the world did we get %#v", there)
	}
	if there[0] != "there" {
		t.Fatalf("why in the world did we not get 'there'?")
	}
}

func TestLocationsFromMapMultiple(t *testing.T) {
	locs := []string{"there", "here"}
	m := map[string]interface{}{"location": "there", "locations": locs}
	there, err := getLocationsFromMap(nil, m)
	if err != nil {
		t.Fatalf("shouldn't have reported error %v", err)
	}
	if len(there) != 3 {
		t.Fatalf("why in the world did we get %#v", there)
	}
}

func TestCodeQueryFromMapBad(t *testing.T) {
	m := map[string]interface{}{"code": 42}
	_, _, err := CodeQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestCodeQueryFromMapGoodSimple(t *testing.T) {
	m := map[string]interface{}{"code": "1+2"}
	_, _, err := CodeQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestCodeQueryFromMapGoodArray1(t *testing.T) {
	m := map[string]interface{}{"code": []string{"var x = 1;", "x + 1;"}}
	_, _, err := CodeQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestCodeQueryFromMapGoodArray2(t *testing.T) {
	m := map[string]interface{}{"code": []interface{}{"var x = 1;", "x + 1;"}}
	_, _, err := CodeQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestCodeQueryFromMapBadArray(t *testing.T) {
	m := map[string]interface{}{"code": []interface{}{"var x = 1;", 42}}
	q, _, err := CodeQueryFromMap(nil, m)
	if err == nil {
		t.Fatalf("should have reported an error (%#v)", q)
	}
}

func TestPatternQueryFromMapBadSimple(t *testing.T) {
	m := map[string]interface{}{"pattern": 42}
	_, _, err := PatternQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestPatternQueryFromMapGood(t *testing.T) {
	pattern := map[string]interface{}{"foo": "bar"}
	m := map[string]interface{}{"pattern": pattern}
	_, _, err := PatternQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestAndQueryFromMapBad1(t *testing.T) {
	m := map[string]interface{}{"and": 42}
	_, _, err := AndQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestAndQueryFromMapBad2(t *testing.T) {
	args := []interface{}{42}
	m := map[string]interface{}{"and": args}
	_, _, err := AndQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestAndQueryFromMapGood(t *testing.T) {
	m := map[string]interface{}{"and": []interface{}{}}
	_, _, err := AndQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestOrQueryFromMapBad1(t *testing.T) {
	m := map[string]interface{}{"or": 42}
	_, _, err := OrQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestOrQueryFromMapBad2(t *testing.T) {
	args := []interface{}{42}
	m := map[string]interface{}{"or": args}
	_, _, err := OrQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}

func TestOrQueryFromMapBad3(t *testing.T) {
	m := map[string]interface{}{"or": []interface{}{}, "shortCircuit": "malformed"}
	_, _, err := OrQueryFromMap(nil, m)
	if err == nil {
		t.Fatal("should have reported an error")
	}
}


func TestOrQueryFromMapGood1(t *testing.T) {
	m := map[string]interface{}{"or": []interface{}{}}
	_, _, err := OrQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}

func TestOrQueryFromMapGood2(t *testing.T) {
	m := map[string]interface{}{"or": []interface{}{}, "shortCircuit": false}
	_, _, err := OrQueryFromMap(nil, m)
	if err != nil {
		t.Fatal("shouldn't have reported an error")
	}
}
