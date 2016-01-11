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
	"testing"
)

func pairFromStrings(k, v string) *Pair {
	return &Pair{[]byte(k), []byte(v)}
}

func TestMemStorageAdd(t *testing.T) {
	ctx := NewContext("TestMemStorageAdd")
	s, _ := NewMemStorage(ctx)
	p := pairFromStrings("likes", "tacos")
	loc := "here"
	if err := s.Add(ctx, loc, p); err != nil {
		t.Fatal(err)
	}
}

func testMemStorageLoad(t *testing.T, ctx *Context, s *MemStorage, loc string) {
	p := pairFromStrings("likes", "tacos")
	if err := s.Add(ctx, loc, p); err != nil {
		t.Fatal(err)
	}

	p = pairFromStrings("wants", "chips")
	if err := s.Add(ctx, loc, p); err != nil {
		t.Fatal(err)
	}

	ps, err := s.Load(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}

	if len(ps) != 2 {
		t.Fatalf("unexpected pairs %#v", ps)
	}
}

func TestMemStorageLoad(t *testing.T) {
	ctx := NewContext("TestMemStorageLoad")
	s, _ := NewMemStorage(ctx)
	testMemStorageLoad(t, ctx, s, "here")
}

func TestMemStorageRem(t *testing.T) {
	loc := "here"
	ctx := NewContext("TestMemStorageRem")
	s, _ := NewMemStorage(ctx)
	testMemStorageLoad(t, ctx, s, loc)

	id := "likes"
	if _, err := s.Remove(ctx, loc, []byte(id)); err != nil {
		t.Fatal(err)
	}

	ps, err := s.Load(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}

	if len(ps) != 1 {
		t.Fatalf("unexpected pairs %#v", ps)
	}
}

func TestMemStorageClear(t *testing.T) {
	ctx := NewContext("TestMemStorageClear")
	s, _ := NewMemStorage(ctx)

	loc := "here"
	testMemStorageLoad(t, ctx, s, loc)

	if _, err := s.Clear(ctx, loc); err != nil {
		t.Fatal(err)
	}

	ps, err := s.Load(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}

	if len(ps) != 0 {
		t.Fatalf("unexpected pairs %#v", ps)
	}
}

func TestMemStorageLocations(t *testing.T) {
	ctx := NewContext("TestMemStorageLocations")
	s, _ := NewMemStorage(ctx)

	loc1 := "here"
	loc2 := "there"

	testMemStorageLoad(t, ctx, s, loc1)
	testMemStorageLoad(t, ctx, s, loc2)

	p := pairFromStrings("wants", "queso")
	if err := s.Add(ctx, loc1, p); err != nil {
		t.Fatal(err)
	}

	wanted := func(loc string) string {
		ps, err := s.Load(ctx, loc)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range ps {
			if string(p.K) == "wants" {
				return string(p.V)
			}
		}
		return "missing"
	}

	if wanted(loc1) != "queso" {
		t.Fatal("didn't want queso")
	}

	if wanted(loc2) != "chips" {
		t.Fatal("didn't want chips")
	}

}

func TestMemStorageMisc(t *testing.T) {
	ctx := NewContext("TestMemStorageMisc")
	s, _ := NewMemStorage(ctx)

	loc := "here"
	testMemStorageLoad(t, ctx, s, loc)

	// GetStats doesn't do anything.
	if _, err := s.GetStats(ctx, loc); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, loc); err != nil {
		t.Fatal(err)
	}

	ps, err := s.Load(ctx, loc)
	if err != nil {
		t.Fatal(err)
	}

	if len(ps) != 0 {
		t.Fatalf("unexpected pairs %#v", ps)
	}

	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkMemStorageAdd(b *testing.B) {
	ctx := BenchContext("BenchmarkMemStorageAdd")
	s, _ := NewMemStorage(ctx)
	loc := "here"

	nPairs := 32
	ps := make([]*Pair, 0, nPairs)
	for i := 0; i < nPairs; i++ {
		ps = append(ps, pairFromStrings(fmt.Sprintf("likes_%d", i), "tacos"))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := ps[i%nPairs]
		if err := s.Add(ctx, loc, p); err != nil {
			b.Fatal(err)
		}
	}

	pairs, err := s.Load(ctx, loc)
	if err != nil {
		b.Fatal(err)
	}

	expected := nPairs
	if b.N < nPairs {
		expected = b.N
	}

	if len(pairs) != expected {
		b.Fatalf("unexpected pairs %d", len(pairs))
	}

}
