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

package bolt

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	. "github.com/Comcast/rulio/core"
)

// TestStorage verifies we can add Pairs and retrieve them using Load
func TestStorage(t *testing.T) {
	ctx := NewContext("boltTest")
	dir, err := ioutil.TempDir("", "boltTest")
	if err != nil {
		t.Fatal("cannot create tempdir", err)
	}
	defer os.RemoveAll(dir)

	b, err := NewStorage(ctx, path.Join(dir, "bolt.db"))
	if err != nil {
		t.Fatal("cannot initialize bolt", err)
	}
	t.Log("created bolt storage on", b.Filename)

	defer b.Destroy(ctx)

	loc := "home"
	err = b.Add(ctx, loc, &Pair{[]byte("eats"), []byte("tacos")})
	if err != nil {
		t.Fatal("cannot add a pair to location", loc, err)
	}
	t.Log("added pair")

	pairs, err := b.Load(ctx, loc)
	if len(pairs) != 1 {
		t.Fatal("expected 1 pair on Load but found %d %+v", len(pairs), pairs)
	}
	t.Log("pair returned on load", b.Filename)

	err = b.Add(ctx, loc, &Pair{[]byte("drinks"), []byte("tequila")})
	if err != nil {
		t.Fatal("cannot add a 2nd pair to location", loc, err)
	}
	t.Log("added 2nd pair")

	pairs, err = b.Load(ctx, loc)
	if len(pairs) != 2 {
		t.Fatal("expected 2 pairs on load but found %d %+v", len(pairs), pairs)
	}
	t.Log("two pairs returned on Load")
}

func benchSetup(b *testing.B) (*BoltStorage, *Context) {
	ctx := BenchContext("boltTest")
	dir, err := ioutil.TempDir("", "boltTest")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	bolt, err := NewStorage(ctx, path.Join(dir, "bolt.db"))
	if err != nil {
		b.Fatal(err)
	}

	return bolt, ctx
}

func addSome(ctx *Context, bolt *BoltStorage, b *testing.B, n int, m int) {
	loc := "home"
	eatss := make([][]byte, 0, m)
	for i := 0; i < m; i++ {
		eatss = append(eatss, []byte(fmt.Sprintf("eats_%d", i)))
	}
	tacos := []byte("tacos")

	b.ResetTimer()

	for i := 0; i < n; i++ {
		if err := bolt.Add(ctx, loc, &Pair{eatss[i%m], tacos}); err != nil {
			b.Fatal(b)
		}
	}

}

func BenchmarkAdd(b *testing.B) {
	bolt, ctx := benchSetup(b)
	defer bolt.Destroy(ctx)

	addSome(ctx, bolt, b, b.N, 16)
}

func BenchmarkLoad(b *testing.B) {
	bolt, ctx := benchSetup(b)
	defer bolt.Destroy(ctx)

	addSome(ctx, bolt, b, 32, 16)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pairs, err := bolt.Load(ctx, "home")
		if err != nil {
			b.Fatal(err)
		}
		if len(pairs) != 16 {
			b.Fatalf("didn't expect %d pairs", len(pairs))
		}
	}
}
