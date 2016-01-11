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
	"errors"
	"fmt"
	"strconv"
	"testing"
)

func SearchTermsTest(ctx *Context, ti *TermIndex, terms []string, expected []string) error {
	ids, err := ti.Search(ctx, terms)
	if err != nil {
		return err
	}
	got := NewStringSet(ids)

	exp := NewStringSet(expected)
	fmt.Printf("Expecting %v\n", exp.Array())
	left, right := exp.Difference(got)

	fmt.Printf("Left %v\n", left.Array())
	fmt.Printf("Right %v\n", right.Array())

	ok := true
	if len(left) > 0 {
		fmt.Printf("Missed %v\n", left.Array())
		ok = false
	}
	if len(right) > 0 {
		fmt.Printf("Extra %v\n", right.Array())

		ok = false
	}

	if ok {
		fmt.Printf("SearchTermsTest passed %v\n", terms)
		return nil
	} else {
		fmt.Printf("SearchTermsTest failed %v\n", terms)
		fmt.Printf("SearchTermsTest expected %v\n", exp.json())
		fmt.Printf("SearchTermsTest got %v\n", got.json())
		return errors.New("SearchTest failed: see the logs.")
	}
}

func TestTermIndexTrivial(t *testing.T) {
	ctx := NewContext("TestTermIndex")

	ti := NewTermIndex()
	ti.Add(ctx, "homer", "1")
	ti.Add(ctx, "bart", "1")
	ti.Add(ctx, "homer", "2")

	err := SearchTermsTest(ctx, ti, []string{"homer"}, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTermIndexBasic(t *testing.T) {
	ctx := NewContext("TestTermIndex")

	ti := NewTermIndex()
	ti.Add(ctx, "homer", "H")
	ti.Add(ctx, "homer", "1")
	ti.Add(ctx, "homer", "2")
	ti.Add(ctx, "homer", "3")
	ti.Add(ctx, "lisa", "L")
	ti.Add(ctx, "lisa", "1")
	ti.Add(ctx, "bart", "B")
	ti.Add(ctx, "marge", "M")
	ti.Add(ctx, "marge", "2")
	ti.Add(ctx, "marge", "3")
	ti.Add(ctx, "marge", "4")

	err := SearchTermsTest(ctx, ti, []string{"homer", "marge"}, []string{"2", "3"})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("term index metrics %v", *(ti.SlowMetrics(ctx)))

}

func TestTermIndexEmptySearch(t *testing.T) {
	ti := NewTermIndex()
	_, err := ti.Search(nil, []string{})
	if err == nil {
		t.Fatal(err)
	}
}

func TestTermIndexNewTerm(t *testing.T) {
	ti := NewTermIndex()
	ti.Add(nil, "homer", "H")
	bss, err := ti.Search(nil, []string{"tacos"})
	if 0 != len(bss) || err != nil {
		t.Fatal(err)
	}
}

func TestTermIndexTermOrder(t *testing.T) {
	ti := NewTermIndex()
	ti.Add(nil, "homer", "H")
	ti.Add(nil, "lisa", "H")
	ti.Add(nil, "bart", "H")
	ti.Add(nil, "homer", "I")
	ti.Add(nil, "lisa", "I")
	bss, err := ti.Search(nil, []string{"homer", "bart", "lisa"})
	if 1 != len(bss) || err != nil {
		t.Fatal(err)
	}
}

func TestTermIndexRem(t *testing.T) {
	ctx := NewContext("TestTermIndex")

	ti := NewTermIndex()
	ti.Add(ctx, "homer", "1")
	ti.Add(ctx, "bart", "1")
	ti.Add(ctx, "lisa", "1")
	ti.Add(ctx, "homer", "2")
	ti.Add(ctx, "bart", "2")
	ti.Add(ctx, "lisa", "2")
	ti.Add(ctx, "bart", "3")
	ti.Add(ctx, "lisa", "3")
	ss, err := ti.Search(nil, []string{"homer", "bart", "lisa"})
	if 2 != len(ss) || err != nil {
		t.Errorf("first search: %v (err %v)", ss, err)
	}

	ti.RemID(ctx, "2")
	ss, err = ti.Search(nil, []string{"homer", "bart", "lisa"})
	if 1 != len(ss) || err != nil {
		t.Errorf("second search: %v (err %v)", ss, err)
	}

	ti.RemIdTerms(ctx, []string{"homer", "bart", "lisa"}, "1")
	ss, err = ti.Search(nil, []string{"homer", "bart", "lisa"})
	if 0 != len(ss) || err != nil {
		t.Errorf("third search: %v (err %v)", ss, err)
	}
	ss, err = ti.Search(nil, []string{"bart", "lisa"})
	if 1 != len(ss) || err != nil {
		t.Errorf("fourth search: %v (err %v)", ss, err)
	}

	ti.Rem(ctx, "newterm", "1") // No op
	if 1 != len(ss) || err != nil {
		t.Errorf("fifth search: %v (err %v)", ss, err)
	}
}

func TestTermNewToken(t *testing.T) {
	ctx := NewContext("TestTermIndex")

	ti := NewTermIndex()
	ti.Add(ctx, "homer", "1")
	ti.Add(ctx, "bart", "1")
	ti.Add(ctx, "lisa", "1")
	ti.Add(ctx, "homer", "2")
	ti.Add(ctx, "bart", "2")
	ti.Add(ctx, "lisa", "2")
	ti.Add(ctx, "bart", "3")
	ti.Add(ctx, "lisa", "3")
	ss, err := ti.Search(nil, []string{"homer", "bart", "lisa", "nobody"})
	if 0 != len(ss) || err != nil {
		t.Errorf("first search: %v (err %v)", ss, err)
	}
}

func BenchmarkTermIndexAdd(b *testing.B) {
	// Not a good benchmark because the times will increase
	// somewhat with the number of Adds.  Could rewrite to have
	// each operation do a fixed number of adds.

	termSpace := 100
	ctx := BenchContext("TermIndexAdd")

	docs := make([][]string, 0, 0)
	for i := 0; i < b.N; i++ {
		nTerms := 5
		doc := make([]string, 0, nTerms+1)
		doc = append(doc, "id"+strconv.Itoa(i))
		for j := 0; j < nTerms; j++ {
			term := strconv.Itoa(i % termSpace)
			doc = append(doc, term)
		}
		docs = append(docs, doc)
	}
	b.ResetTimer()
	ti := NewTermIndex()
	for i := 0; i < b.N; i++ {
		doc := docs[i]
		id := doc[0]
		for j := 1; j < len(doc); j++ {
			ti.Add(ctx, doc[j], id)
		}
	}
}

func BenchmarkTermIndexSearch(b *testing.B) {
	ctx := BenchContext("TermIndexAdd")
	termSpace := 100
	numDocs := 1000
	numTermsPerDoc := 5
	ti := NewTermIndex()
	for i := 0; i < numDocs; i++ {
		id := "id" + strconv.Itoa(i)
		for j := 0; j < numTermsPerDoc; j++ {
			term := strconv.Itoa((i * j) % termSpace)
			ti.Add(ctx, term, id)
		}

	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		term1 := strconv.Itoa(i % termSpace)
		term2 := strconv.Itoa((i + 1) % termSpace)
		term3 := strconv.Itoa((i + 2) % termSpace)
		_, err := ti.Search(ctx, []string{term1, term2, term3})
		if err != nil {
			b.Fail()
		}
	}
}
