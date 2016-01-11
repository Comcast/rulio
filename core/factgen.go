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
	"math/rand"
	"testing"
	"time"
)

const (
	// These numbers shouldn't be constants, and they should be much higher.
	kspace = 10 // Represents the range of map keys
	vspace = 50 // Represents the range of values
)

func RandVar() string {
	sd := vspace / 10.0
	mean := vspace / 5.0
	n := int(rand.NormFloat64()*sd + mean)
	if n < 0 {
		n = 0
	} else if vspace <= n {
		n = vspace - 1
	}
	return fmt.Sprintf("v%d", n)
}

// Generate a random Fact.  'Width' is minimum number of properties.
// 'Delta' indicates a random number of additional properties.
// `Depth` specifies the depth of the structure of the values.  If
// 'makeVar' is true, then about 1/3 of the values will be variables.
func genFact(ctx *Context, width int, delta int, depth int, makeVar bool) Map {

	f := make(Map)
	w := rand.Intn(delta) + width
	for i := 0; i < w; i++ {
		k := fmt.Sprintf("k%d", rand.Intn(kspace))
		c := rand.Intn(4)
		if c == 0 && 1 < depth {
			f[k] = genFact(ctx, width, delta, depth-1, makeVar)
			continue
		}
		v := RandVar()
		if c < 2 {
			f[k] = interface{}(v)
			continue
		}

		if makeVar {
			f[k] = interface{}("?" + v)
			continue
		}

		f[k] = interface{}(v)
	}
	return f
}

func genFacts(ctx *Context, n int, width int, delta int, depth int, makeVar bool) []Map {
	facts := make([]Map, 0, n)
	for i := 0; i < n; i++ {
		facts = append(facts, genFact(ctx, width, delta, depth, makeVar))
	}
	return facts
}

func RandomFactsTest(ctx *Context, numberOfFacts int, numberOfQueries int, t *testing.T) (int64, error) {

	t.Logf("RandomFactsTest facts %d queries %d", numberOfFacts, numberOfQueries)

	rand.Seed(Now())

	loc, _ := NewLocation(ctx, "random", nil, nil)

	then := Now()

	for _, f := range genFacts(ctx, numberOfFacts, 4, 3, 2, false) {
		if _, err := loc.AddFact(ctx, "", Map(f)); err != nil {
			t.Fatal(err)
		}

	}

	for i, f := range genFacts(ctx, numberOfQueries, 4, 3, 2, true) {
		srs, err := loc.state.Search(ctx, f)
		if err != nil {
			return 0, err
		}
		t.Logf("TestRandomFacts %v %d %#v\n", i, len(srs.Found), f)
	}

	elapsed := int64((time.Now().UTC().UnixNano() - then) / 1000000)

	t.Logf("RandomFactsTest elapsed %d", elapsed)

	return elapsed, nil
}
