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

func indexedState(t *testing.T) (*Context, *IndexedState) {
	ctx := BenchContext("test")
	store, _ := NewMemStorage(ctx)
	s, err := NewIndexedState(ctx, "test", store)
	if err != nil {
		if t == nil {
			panic(err)
		} else {
			t.Fatal(err)
		}
	}
	return ctx, s
}

func TestIndexedStateBasic(t *testing.T) {
	ctx, s := indexedState(t)
	testStateBasic(t, ctx, s)
}

func BenchmarkIndexedStateAdd(b *testing.B) {
	ctx, s := indexedState(nil)
	benchStateAdd(b, ctx, s)
}

func BenchmarkIndexedStateSearch(b *testing.B) {
	ctx, s := indexedState(nil)
	benchStateSearch(b, ctx, s)
}

func TestIndexedStatePropertiesBasic(t *testing.T) {
	ctx, s := indexedState(t)
	testStatePropertiesBasic(t, ctx, s)
}

func BenchmarkIndexedStatePropertiesBasic(b *testing.B) {
	ctx, s := indexedState(nil)
	benchStatePropertiesBasic(b, ctx, s)
}

func TestIndexedStateIdPropertiesBasic(t *testing.T) {
	ctx, s := indexedState(t)
	testStateIdPropertiesBasic(t, ctx, s)
}

func TestIndexedStateIdPropertiesOverwrite(t *testing.T) {
	ctx, s := indexedState(t)
	testStateIdPropertiesOverwrite(t, ctx, s)
}

func TestIndexedStateDeleteDependencies(t *testing.T) {
	ctx, s := indexedState(t)
	testDeleteStateDependencies(t, ctx, s)
}

func TestIndexedStateExpires(t *testing.T) {
	testStateExpires(t, func(ctx *Context, store Storage, loc string) (State, error) {
		return NewIndexedState(ctx, loc, store)
	})
}

func TestIndexedStateTTL(t *testing.T) {
	ctx, s := indexedState(t)
	testStateTTL(t, ctx, s)
}

func TestIndexedStateProptTTL(t *testing.T) {
	ctx, s := indexedState(t)
	testStatePropTTL(t, ctx, s)
}

func TestIndexedStateRuleTTL(t *testing.T) {
	ctx, s := indexedState(t)
	testStateRuleTTL(t, ctx, s)
}
