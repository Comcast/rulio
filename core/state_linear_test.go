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
	"testing"
)

func linearState(t *testing.T) (*Context, *LinearState) {
	ctx := BenchContext("test")
	store, _ := NewMemStorage(ctx)
	s, err := NewLinearState(ctx, "test", store)
	if err != nil {
		if t == nil {
			panic(err)
		} else {
			t.Fatal(err)
		}
	}
	return ctx, s
}

func TestLinearStateBasic(t *testing.T) {
	ctx, s := linearState(t)
	testStateBasic(t, ctx, s)
}

func BenchmarkLinearStateAdd(b *testing.B) {
	ctx, s := linearState(nil)
	benchStateAdd(b, ctx, s)
}

func BenchmarkLinearStateSearch(b *testing.B) {
	ctx, s := linearState(nil)
	benchStateSearch(b, ctx, s)
}

func TestLinearStatePropertiesBasic(t *testing.T) {
	ctx, s := linearState(t)
	testStatePropertiesBasic(t, ctx, s)
}

func TestLinearStateIdPropertiesBasic(t *testing.T) {
	ctx, s := linearState(t)
	testStateIdPropertiesBasic(t, ctx, s)
}

func TestLinearStateIdPropertiesOverwrite(t *testing.T) {
	ctx, s := linearState(t)
	testStateIdPropertiesOverwrite(t, ctx, s)
}

func TestLinearStateDeleteDependencies(t *testing.T) {
	ctx, s := linearState(t)
	testDeleteStateDependencies(t, ctx, s)
}

func TestLinearStateCount(t *testing.T) {
	ctx, s := linearState(t)

	if err := s.Load(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := s.Add(ctx, "i1", mapJS(`{"likes":"tacos"}`))
	if err != nil {
		t.Fatal(err)
	}
	if n := s.Count(ctx); n != 1 {
		t.Fatalf("bad count: %d", n)
	}

	if !s.IsLoaded(ctx) {
		t.Fatal("state really is loaded")
	}

	if _, err = s.Rem(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	if n := s.Count(ctx); n != 0 {
		t.Fatalf("bad count after rem: %d", n)
	}
}

func TestLinearStateLoad(t *testing.T) {
	// First we write stuff through a state to a store.
	// The we create a new state pointed at the old store.

	ctx := NewContext("test")
	store, _ := NewMemStorage(ctx)

	{
		s, err := NewLinearState(ctx, "test", store)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Load(ctx); err != nil {
			t.Fatal(err)
		}

		_, err = s.Add(ctx, "i1", mapJS(`{"likes":"tacos"}`))
		if err != nil {
			t.Fatal(err)
		}
	}

	{
		s, err := NewLinearState(ctx, "test", store)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Load(ctx); err != nil {
			t.Fatal(err)
		}
		if n := s.Count(ctx); n != 1 {
			t.Fatalf("bad count: %d", n)
		}
	}
}

func TestLinearStateExpires(t *testing.T) {
	testStateExpires(t, func(ctx *Context, store Storage, loc string) (State, error) {
		return NewLinearState(ctx, loc, store)
	})
}

func TestLinearStateTTL(t *testing.T) {
	ctx, s := linearState(t)
	testStateTTL(t, ctx, s)
}

func TestLinearStateBadTTL(t *testing.T) {
	ctx, s := linearState(t)
	testStateBadTTL(t, ctx, s)
}

func TestLinearStatePropTTL(t *testing.T) {
	ctx, s := linearState(t)
	testStatePropTTL(t, ctx, s)
}

func TestLinearStateRuleTTL(t *testing.T) {
	ctx, s := linearState(t)
	testStateRuleTTL(t, ctx, s)
}
