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
	"testing"
	"time"
)

func checkInjectedId(t *testing.T, fact string, id string) {
	if SystemParameters.IdInjectionTime == InjectIdNever {
		return // I guess.
	}

	var m map[string]interface{}
	err := json.Unmarshal([]byte(fact), &m)
	if err != nil {
		t.Fatal(err)
	}
	given, have := m[KW_id]
	if !have {
		t.Fatalf("didn't find an id in %s", fact)
	}
	s, ok := given.(string)
	if !ok {
		t.Fatalf("id %#v is a %T", given, given)
	}
	if s != id {
		t.Fatalf("wanted id '%s' but got '%s'", id, s)
	}

	fmt.Printf("checkInjectedId wanted '%s' and got '%s'\n", id, s)
}

func benchStateAdd(b *testing.B, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Add(ctx, "i1", mapJS(`{"likes":"tacos"}`))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchStateSearch(b *testing.B, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		b.Fatal(err)
	}
	_, err := s.Add(ctx, "i1", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func testStateBasic(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	// Write  fact and then overwrite that fact.

	id, err := s.Add(ctx, "i1", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "i1" {
		t.Fatalf("unexpected id '%s'", id)
	}

	id, err = s.Add(ctx, "i1", mapJS(`{"likes":"beer"}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "i1" {
		t.Fatalf("unexpected id '%s'", id)
	}

	{

		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 1 {
			t.Fatalf("unexpected %d results found", len(srs.Found))
		}
		sr := srs.Found[0]
		if len(sr.Bindingss) != 1 {
			t.Fatalf("unexpected %d bindings found", len(sr.Bindingss))
		}

		bs := sr.Bindingss[0]
		x, have := bs["?x"]
		if !have {
			t.Fatal("failed to find binding for '?x'")
		}
		switch vv := x.(type) {
		case string:
			if vv != "beer" {
				t.Fatalf("expected beer, not %s", vv)
			}
		default:
			t.Fatalf("didn't expect a %T", x)
		}

		checkInjectedId(t, sr.Js, id)
	}

	// Add another fact.
	id, err = s.Add(ctx, "i2", mapJS(`{"likes":"chips"}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "i2" {
		t.Fatalf("unexpected id '%s'", id)
	}

	// And another.
	id, err = s.Add(ctx, "i3", mapJS(`{"wants":"chips"}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "i3" {
		t.Fatalf("unexpected id '%s'", id)
	}

	{

		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 2 {
			t.Fatalf("unexpected %d results found", len(srs.Found))
		}
		for _, sr := range srs.Found {
			if len(sr.Bindingss) != 1 {
				t.Fatalf("unexpected %d bindings found", len(sr.Bindingss))
			}
			bs := sr.Bindingss[0]
			_, have := bs["?x"]
			if !have {
				t.Fatal("failed to find binding for '?x'")
			}
			checkInjectedId(t, sr.Js, sr.Id)
		}
	}

	// Write a rule.
	id, err = s.Add(ctx, "r1", mapJS(`{"rule":{"when":{"pattern":{"opens":"?x"}}, "action":"console.log('hello')"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "r1" {
		t.Fatalf("unexpected rule id '%s'", id)
	}

	// Write another rule.
	id, err = s.Add(ctx, "r2", mapJS(`{"rule":{"when":{"pattern":{"closes":"?x"}}, "action":"console.log('goodbye')"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if id != "r2" {
		t.Fatalf("unexpected rule id '%s'", id)
	}

	// Find a rule for an event.
	event := `{"opens":"door"}`
	m := make(map[string]interface{})
	err = json.Unmarshal([]byte(event), &m)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := s.FindRules(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("unexpected %d rules", len(rs))
	}
	for id, _ := range rs {
		if id != "r1" {
			t.Fatalf("unexpected rule id '%s'", id)
		}
		// We don't inject the id into the rule itself.
		// We could at during RuleFromMap.

		// checkInjectedId(t, toJSON(rule), id)
	}
}

func testStatePropertiesBasic(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := SetProp(ctx, s, "", "likes", "tacos")
	if err != nil {
		t.Fatal(err)
	}
	val, have, err := GetPropString(ctx, s, "likes", "chips")
	if err != nil {
		t.Fatal(err)
	}
	if !have {
		t.Fatal("lost key")
	}
	if val != "tacos" {
		t.Fatal(fmt.Sprintf("(got) %s != %s (wanted)", val, "tacos"))
	}

	found, err := RemProp(ctx, s, "", "likes")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("Should have found the key during RemProp")
	}

	x, have, err := GetPropString(ctx, s, "likes", "beer")
	if err != nil {
		t.Fatal(err)
	}
	if have {
		t.Fatalf("should not have property (%#v) after RemProp", x)
	}
}

func testStateIdPropertiesBasic(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	id, err := s.Add(ctx, "", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fact id: %s", id)

	// Write an irrelevant property for fun.
	pid, err := SetProp(ctx, s, id, "hater", "marge")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %s", pid)

	pid, err = SetProp(ctx, s, id, "author", "homer")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %s", pid)

	val, have, err := GetProp(ctx, s, id, "author", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if !have {
		t.Fatal("lost property")
	}
	if val != "homer" {
		t.Fatal(fmt.Sprintf("%s (%T) != %s (%T)", val, val, "homer", "homer"))
	}

	if _, err := s.Rem(ctx, pid); err != nil {
		t.Fatal(err)
	}

	val, have, err = GetProp(ctx, s, id, "author", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if have {
		t.Fatal("shouldn't have found prop")
	}

	f, err := s.Get(ctx, id)
	if nil != err {
		t.Fatal(err)
	}

	like, have := f["likes"]
	if !have || "tacos" != like {
		t.Fatal("original fact has changed")
	}
}

func benchStatePropertiesBasic(b *testing.B, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		b.Fatal(err)
	}

	_, err := SetProp(ctx, s, "", "likes", "tacos")
	if err != nil {
		b.Fatal(err)
	}

	// Add some other facts.
	for i := 0; i < 20; i++ {
		fact := mapJS(fmt.Sprintf(`{"wants":"beer-%d"}`, 1))
		_, err := s.Add(ctx, "", fact)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, have, err := GetPropString(ctx, s, "likes", "chips")
		if err != nil {
			b.Fatal(err)
		}
		if !have {
			b.Fatal("lost key")
		}
	}
}

func testStateIdPropertiesOverwrite(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	id, err := s.Add(ctx, "", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fact id: %s", id)

	{
		pid, err := SetProp(ctx, s, id, "author", "homer")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("pid: %s", pid)
	}

	// Overwrite previous.

	pid, err := SetProp(ctx, s, id, "author", "bart")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pid: %s", pid)

	val, have, err := GetProp(ctx, s, id, "author", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if !have {
		t.Fatal("lost property")
	}
	if val != "bart" {
		t.Fatal(fmt.Sprintf("%s (%T) != %s (%T)", val, val, "bart", "bart"))
	}

	if _, err := s.Rem(ctx, pid); err != nil {
		t.Fatal(err)
	}

	val, have, err = GetProp(ctx, s, id, "author", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if have {
		t.Fatal("shouldn't have found prop")
	}

	f, err := s.Get(ctx, id)
	if nil != err {
		t.Fatal(err)
	}

	like, have := f["likes"]
	if !have || "tacos" != like {
		t.Fatal("original fact has changed")
	}
}

func testDeleteStateDependencies(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	// Add facts with dependencies.
	id1, err := s.Add(ctx, "id1", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}

	id2, err := s.Add(ctx, "id2", mapJS(`{"likes":"beers", "`+KW_DeleteWith+`":["id1", "id3"]}`))
	if err != nil {
		t.Fatal(err)
	}

	id3, err := s.Add(ctx, "id3", mapJS(`{"likes":"pizza", "`+KW_DeleteWith+`":["id1", "id2"]}`))
	if err != nil {
		t.Fatal(err)
	}

	// Confirm facts
	if f1, err := s.Get(ctx, id1); nil != err {
		t.Fatal(err)
	} else if like, _ := f1["likes"]; "tacos" != like {
		t.Fatal("Fact 1 not found")
	}

	if f2, err := s.Get(ctx, id2); nil != err {
		t.Fatal(err)
	} else if like, _ := f2["likes"]; "beers" != like {
		t.Fatal("Fact 2 not found")
	}

	if f3, err := s.Get(ctx, id3); nil != err {
		t.Fatal(err)
	} else if like, _ := f3["likes"]; "pizza" != like {
		t.Fatal("Fact 3 not found")
	}

	// Delete fact with dependencies.
	if _, err := s.Rem(ctx, id1); nil != err {
		t.Fatal(err)
	}

	// Confirm dependencies are gone.
	_, err = s.Get(ctx, id1)
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Should not find fact 1")
	}

	_, err = s.Get(ctx, id2)
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Should not find fact 2")
	}

	_, err = s.Get(ctx, id3)
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatal("Should not find fact 3")
	}
}

func testStateExpires(t *testing.T, newState func(ctx *Context, store Storage, loc string) (State, error)) {
	ctx := BenchContext("test")

	store, _ := NewMemStorage(ctx)
	loc := "test"
	s, err := newState(ctx, store, loc)

	if err = s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	expires := time.Now().Add(2 * time.Second).UTC().Format(time.RFC3339Nano)

	_, err = s.Add(ctx, "i1", mapJS(fmt.Sprintf(`{"likes":"tacos","expires":"%s"}`, expires)))

	if err != nil {
		t.Fatal(err)
	}

	{
		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 1 {
			t.Fatalf("unexpected %d results found: %#v", len(srs.Found), srs.Found)
		}
	}

	time.Sleep(2500 * time.Millisecond)

	// test search expires
	{
		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 0 {
			t.Fatalf("unexpected %d results found after expiration: %#v", len(srs.Found), srs.Found)
		}

		// store should have 1 fact for IndexedState and 0 fact for LinearState (clear all expired facts during search)
		pairs, err := store.Load(ctx, loc)
		if nil != err {
			t.Fatal(err)
		}
		switch s.(type) {
		case *IndexedState:
			if len(pairs) != 0 {
				t.Fatalf("unexpected %d records found after expiration", len(pairs))
			}
		case *LinearState:
			if len(pairs) != 0 {
				t.Fatalf("unexpected %d records found after expiration", len(pairs))
			}
		}
	}

	// test load expires
	s, _ = newState(ctx, store, loc)
	if err = s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// store should have no facts
	pairs, err := store.Load(ctx, loc)
	if nil != err {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Fatalf("unexpected %d records found after expiration", len(pairs))
	}
}

func testStateTTL(t *testing.T, ctx *Context, s State) {
	var err error
	if err = s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = s.Add(ctx, "i1", mapJS(`{"likes":"tacos","ttl":"2s"}`))
	if err != nil {
		t.Fatal(err)
	}

	{
		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 1 {
			t.Fatalf("unexpected %d results found: %#v", len(srs.Found), srs.Found)
		}
	}

	time.Sleep(2500 * time.Millisecond)

	{
		srs, err := s.Search(ctx, mapJS(`{"likes":"?x"}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(srs.Found) != 0 {
			t.Fatalf("unexpected %d results found after expiration: %#v", len(srs.Found), srs.Found)
		}
	}
}

func testStateBadTTL(t *testing.T, ctx *Context, s State) {
	var err error
	if err = s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = s.Add(ctx, "i1", mapJS(`{"likes":"tacos","ttl":"tacos"}`))
	if err == nil {
		t.Fatal(err)
	}
}

func testStatePropTTL(t *testing.T, ctx *Context, s State) {
	var err error
	if err = s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = s.Add(ctx, "", mapJS(`{"!loves":"tacos","ttl":"2s"}`))
	if err != nil {
		t.Fatal(err)
	}

	_, have, err := GetProp(ctx, s, "", "loves", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if !have {
		t.Fatal("should have found prop")
	}

	time.Sleep(3 * time.Second)

	_, have, err = GetProp(ctx, s, "", "loves", "NA")
	if err != nil {
		t.Fatal(err)
	}
	if have {
		t.Fatal("shouldn't have found prop after expiration")
	}
}

func testStateRuleTTL(t *testing.T, ctx *Context, s State) {
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := s.Add(ctx, "r1", mapJS(`{"ttl":"2s", "rule":{"when":{"pattern":{"opens":"?x"}}, "action":"console.log('hello')"}}`))
	if err != nil {
		t.Fatal(err)
	}

	id, err := s.Add(ctx, "r2", mapJS(`{"rule":{"when":{"pattern":{"opens":"?x"}}, "action":"console.log('hola')"}}`))
	if err != nil {
		t.Fatal(err)
	}

	// Wait around.
	time.Sleep(2500 * time.Millisecond)

	// Find a rule for an event.
	event := `{"opens":"door"}`
	m := make(map[string]interface{})
	err = json.Unmarshal([]byte(event), &m)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := s.FindRules(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("unexpected %d rules", len(rs))
	}
	for rid, _ := range rs {
		if rid != id {
			t.Fatalf("unexpected rule id '%s'", id)
		}
	}
}
