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
)

func TestSubstituteBindings(t *testing.T) {
	code := `{"a":1,"b":"?b","c":"?c","d":["?b","?c"]}`
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(code), &m); err != nil {
		t.Fatal(err)
	}

	bs := make(map[string]interface{})
	bs["?b"] = "homer"
	bs["?c"] = "cake"

	got, err := SubstituteBindings(nil, code, bs)
	if err != nil {
		t.Fatal(err)
	}
	expect := `{"a":1,"b":"homer","c":"cake","d":["homer","cake"]}`

	js, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	// Important: json.Marshal sorts keys.

	s := string(js)
	if s != expect {
		t.Fatal(fmt.Sprintf("%s != %s", expect, s))
	}
}

func TestNakedVariable(t *testing.T) {
	tests := map[string]bool{
		"?foo":       true,
		"?foo bar":   false,
		" ?foo":      false,
		"?foo ":      false,
		"foo":        false,
		"?FooBar":    true,
		"?foo42":     true,
		"?foo42.bar": false,
	}
	for s, b := range tests {
		if IsNakedVariable(s) != b {
			t.Fatal(b)
		}
	}
}
func TestSubstituteBindingsUnbound(t *testing.T) {

	ctx := TestContext("Test")
	loc, err := NewLocation(ctx, "test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctl := &Control{
		UseDefaultVariableValue: true,
		DefaultVariableValue:    "undefined",
	}
	loc.SetControl(ctl)

	bs := make(Bindings)
	bs["?likes"] = "tacos"
	code := `{"perhaps":{"wants":"?likes","has":"?unbound"}}`

	x, err := SubstituteBindings(ctx, code, bs)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("x=%#v", x)

	ctl = &Control{
		UseDefaultVariableValue: false,
		DefaultVariableValue:    "undefined",
	}
	loc.SetControl(ctl)

	x, err = SubstituteBindings(ctx, code, bs)
	if err == nil {
		t.Fatal("should have seen an error for the unbound variable")
	}

	fmt.Printf("x=%#v", x)
}

func TestOttoActionInterpreter(t *testing.T) {
	bs := Bindings(map[string]interface{}{"x": 1, "y": 2})
	a := Action{
		Code:     "x+y+3",
		Endpoint: "otto",
	}
	want := 1 + 2 + 3

	oai := &OttoActionInterpreter{}

	name := "test"
	ctx := NewContext(name)
	store, _ := NewMemStorage(ctx)
	state, _ := NewIndexedState(ctx, name, store)
	loc, err := NewLocation(ctx, "test", state, nil)
	if err != nil {
		t.Fatal(err)
	}
	f, err := oai.GetThunk(ctx, loc, bs, a)
	if err != nil {
		t.Fatal(err)
	}
	x, err := f()
	if err != nil {
		t.Fatal(err)
	}

	if n, is := x.(float64); is {
		if int(n) == want {
			return
		}
	}
	t.Fatal(fmt.Sprintf("wanted %v but got %#v", want, x))
}

func TestActionInterpreters(t *testing.T) {
	bs := Bindings(map[string]interface{}{"x": 1, "y": 2})
	a := Action{
		Code:     "x+y+3",
		Endpoint: "otto",
	}
	want := 1 + 2 + 3

	oai := &OttoActionInterpreter{}

	name := "test"
	ctx := NewContext(name)
	store, _ := NewMemStorage(ctx)
	state, _ := NewIndexedState(ctx, name, store)
	loc, err := NewLocation(ctx, "test", state, nil)
	if err != nil {
		t.Fatal(err)
	}
	loc.Control().ActionInterpreters = map[string]ActionInterpreter{
		"otto": oai,
	}
	f, err := loc.getActionFunc(ctx, bs, a)
	if err != nil {
		t.Fatal(err)
	}
	x, err := f()
	if err != nil {
		t.Fatal(err)
	}

	if n, is := x.(float64); is {
		if int(n) == want {
			return
		}
	}
	t.Fatal(fmt.Sprintf("wanted %v but got %#v", want, x))
}
