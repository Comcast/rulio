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
	"log"
	"testing"
	"time"
)

func TestLocationControl(t *testing.T) {
	x := Location{}
	// We check both that Control() returns non-nil and that the
	// value has a default MaxFacts.
	if 0 == x.Control().MaxFacts {
		t.Fatal("zero MaxFacts")
	}
}

func TestLocationAddFact(t *testing.T) {
	ctx, loc := TestingLocation(t)

	{
		id, err := loc.AddFact(ctx, "f1", mapJS(`{"likes":"tacos"}`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "f1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	{
		id, err := loc.AddFact(ctx, "f2", mapJS(`{"wants":"chips"}`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "f2" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	srs, err := loc.SearchFacts(ctx, mapJS(`{"likes":"?x"}`), true)

	if err != nil {
		t.Fatal(err)
	}

	if len(srs.Found) != 1 {
		t.Logf("srs %#v", srs)
		t.Fatal(fmt.Errorf("didn't expect %d results", len(srs.Found)))
	}

	sr := srs.Found[0]

	if len(sr.Bindingss) != 1 {
		t.Logf("sr %#v", sr)
		t.Fatal(fmt.Errorf("didn't expect %d bindingss", len(sr.Bindingss)))
	}

	binding, given := sr.Bindingss[0]["?x"]
	if !given {
		t.Logf("bs %#v", sr.Bindingss[0])
		t.Fatal(fmt.Errorf("no binding for x"))
	}

	if binding != "tacos" {
		t.Fatalf("unexpected '%v' binding", binding)
	}
}

func TestLocationAddRule(t *testing.T) {
	ctx, loc := TestingLocation(t)
	c := make(chan interface{})
	ctx.AddValue("out", c)

	{
		id, err := loc.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "action":{"code":"Env.out('somebody wants ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	go func() {
		work, err := loc.ProcessEvent(ctx, mapJS(`{"wants":"beer"}`))
		if err != nil {
			t.Fatal(err)
		}
		log.Printf("work %#v", work)
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(3 * 1e9):
		t.Fatal("timeout")
	}
}

func TestLocationAddParentRule(t *testing.T) {
	ctx, parent := NamedTestingLocation(t, "parent")
	// ctx = TestContext("test")

	c := make(chan interface{})
	ctx.AddValue("out", c)

	{
		id, err := parent.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "action":{"code":"Env.out('somebody wants ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	_, child := NamedTestingLocation(t, "child")

	if err := child.SetProp(ctx, "", "parents", []interface{}{"parent"}); err != nil {
		t.Fatal(err)
	}

	child.Provider = NewSimpleLocationProvider(map[string]*Location{
		"parent": parent,
	})

	check := func(wanted bool) {
		go func() {
			work, err := child.ProcessEvent(ctx, mapJS(`{"wants":"beer"}`))
			if err != nil {
				t.Fatal(err)
			}
			js, e := json.MarshalIndent(work, "  ", "  ")
			if e != nil {
				t.Fatal(err)
			}
			log.Printf("work %s", js)
		}()

		select {
		case heard := <-c:
			if !wanted || heard != "somebody wants beer" {
				t.Fatalf("unexpected message '%s'", heard)
			}
		case <-time.After(3 * 1e9):
			if wanted {
				t.Fatal("timeout")
			}
		}
	}

	check(true)

	if err := child.EnableRule(ctx, "r1", false); err != nil {
		t.Fatal(err)
	}

	check(false)

	if err := child.EnableRule(ctx, "r1", true); err != nil {
		t.Fatal(err)
	}

	check(true)

	// Now check that we detect duplicate ids.  Note that we don't
	// check when a rule is created, but maybe we should.

	{
		id, err := child.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "action":{"code":"Env.out('somebody wants ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}

		if _, err = child.ProcessEvent(ctx, mapJS(`{"wants":"beer"}`)); err == nil {
			t.Fatal("failed to notice duplicate id")
		}
	}

}

func TestBindingsWarningLimit(t *testing.T) {
	ctx, loc := TestingLocation(t)
	ctx.Verbosity = EVERYTHING
	limit := 5
	loc.Control().BindingsWarningLimit = limit

	for i := 0; i <= limit; i++ {
		_, err := loc.AddFact(ctx, "", mapJS(fmt.Sprintf(`{"likes":"beer%d"}`, i)))
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx.LogAccumulator = NewAccumulator(10000)
	ctx.LogAccumulatorLevel = EVERYTHING

	_, err := loc.SearchFacts(ctx, mapJS(`{"likes":"?x"}`), true)

	if err != nil {
		t.Fatal(err)
	}

	gotWarning := false
	for _, lg := range ctx.LogAccumulator.Acc {
		m, ok := lg.(map[string]interface{})
		if !ok {
			t.Fatalf("bad log record %#v", lg)
		}
		_, have := m["bindingsWarning"]
		if have {
			gotWarning = true
			break
		}
	}
	if !gotWarning {
		t.Fatal("failed to get warning")
	}
}

func TestDeleteLocationMem(t *testing.T) {
	name := "test"
	ctx := NewContext(name)

	store, _ := NewMemStorage(ctx)
	state, _ := NewIndexedState(ctx, name, store)
	loc, err := NewLocation(ctx, "test", state, nil)

	if err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatal(err)
	}

	loc.SetControl(&Control{MaxFacts: 100})

	if err = loc.Clear(ctx); err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatal(err)
	}
	ctx.SetLoc(loc)

	_, err = loc.AddFact(ctx, "f1", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}

	{

		srs, err := loc.SearchFacts(ctx, mapJS(`{"likes":"?x"}`), true)

		if err != nil {
			t.Fatal(err)
		}

		if len(srs.Found) != 1 {
			t.Fatal(err)
		}
	}

	// Get the location again.

	state, _ = NewIndexedState(ctx, name, store)
	loc, err = NewLocation(ctx, "test", state, nil)

	{
		srs, err := loc.SearchFacts(ctx, mapJS(`{"likes":"?x"}`), true)

		if err != nil {
			t.Fatal(err)
		}

		if len(srs.Found) != 1 {
			t.Fatal(err)
		}
	}

	if err = loc.Delete(ctx); err != nil {
		t.Fatal(err)
	}

	state, _ = NewIndexedState(ctx, name, store)
	loc, err = NewLocation(ctx, "test", state, nil)

	{
		srs, err := loc.SearchFacts(ctx, mapJS(`{"likes":"?x"}`), true)

		if err != nil {
			t.Fatal(err)
		}

		if len(srs.Found) != 0 {
			t.Fatal(err)
		}
	}

}

func TestLocationAncestorsLoopingProtection(t *testing.T) {
	ctx, child := NamedTestingLocation(t, "child")

	if err := child.SetProp(ctx, "", "parents", []interface{}{"child"}); err != nil {
		t.Fatal(err)
	}

	child.Provider = NewSimpleLocationProvider(map[string]*Location{
		"child": child,
	})

	terminated := make(chan bool)
	caught := make(chan bool)
	go func() {
		_, cond := child.ProcessEvent(ctx, mapJS(`{"wants":"tacos"}`))
		if cond != nil {
			if cond.Msg == AncestorLoop.Error() {
				caught <- true
				return
			}
		}
		if cond != nil {
			t.Fatal(cond)
		}
		terminated <- true
	}()

	select {
	case <-caught:
	case <-terminated:
		t.Fatal("loop not detected")
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}
