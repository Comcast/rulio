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
	"time"
)

func TestEventBasic(t *testing.T) {
	ctx, loc := TestingLocation(t)
	c := make(chan interface{})
	ctx.AddValue("out", c)

	{
		id, err := loc.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "condition":{"pattern":{"has":"?x"}},
 "action":{"code":"console.log(JSON.stringify(event)); console.log(x); Env.out('somebody wants ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	if _, err := loc.AddFact(ctx, "tacos", mapJS(`{"has":"tacos"}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := loc.AddFact(ctx, "beer", mapJS(`{"has":"beer"}`)); err != nil {
		t.Fatal(err)
	}

	go func() {
		_, err := loc.ProcessEvent(ctx, mapJS(`{"wants":"beer"}`))
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestEventTriggered(t *testing.T) {
	ctx, loc := TestingLocation(t)
	c := make(chan interface{})
	ctx.AddValue("out", c)

	rule := `
{"when":{"pattern":{"wants":"?x"}},
 "condition":{"pattern":{"has":"?x"}},
 "action":{"code":"console.log(JSON.stringify(event)); console.log(x); Env.out('somebody wants ' + x)"}}
`

	id := "r1"

	if _, err := loc.AddRule(ctx, id, mapJS(rule)); err != nil {
		t.Fatal(err)
	}

	if _, err := loc.AddFact(ctx, "tacos", mapJS(`{"has":"tacos"}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := loc.AddFact(ctx, "beer", mapJS(`{"has":"beer"}`)); err != nil {
		t.Fatal(err)
	}

	event := mapJS(fmt.Sprintf(`{"wants":"beer","trigger!":"%s"}`, id))

	go func() {
		_, err := loc.ProcessEvent(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(5 * 1e9):
		t.Fatal("timeout")
	}

}

func TestEventEmbedded(t *testing.T) {
	ctx, loc := TestingLocation(t)
	c := make(chan interface{})
	ctx.AddValue("out", c)

	rule := `
{"when":{"pattern":{"wants":"?x"}},
 "condition":{"pattern":{"has":"?x"}},
 "action":{"code":"console.log(JSON.stringify(event)); console.log(x); Env.out('somebody wants ' + x)"}}
`

	if _, err := loc.AddFact(ctx, "tacos", mapJS(`{"has":"tacos"}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := loc.AddFact(ctx, "beer", mapJS(`{"has":"beer"}`)); err != nil {
		t.Fatal(err)
	}

	event := mapJS(fmt.Sprintf(`{"wants":"beer","evaluate!":%s}`, rule))

	go func() {
		_, err := loc.ProcessEvent(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(5 * 1e9):
		t.Fatal("timeout")
	}

}

func TestEventWithAppBinding(t *testing.T) {
	ctx, loc := TestingLocation(t)
	ctx.App = &BindingApp{
		Bindings: map[string]interface{}{
			"brand": "Duff",
		},
	}

	c := make(chan interface{})
	ctx.AddValue("out", c)

	{
		id, err := loc.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "condition":{"code":"brand == 'Duff'"},
 "action":{"code":"console.log(JSON.stringify(event)); console.log(x); Env.out('somebody wants ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	if _, err := loc.AddFact(ctx, "tacos", mapJS(`{"has":"tacos"}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := loc.AddFact(ctx, "beer", mapJS(`{"has":"beer"}`)); err != nil {
		t.Fatal(err)
	}

	// "I would kill everyone in this room for a drop of sweet beer."
	//
	//   --Homer Simpson

	go func() {
		_, err := loc.ProcessEvent(ctx, mapJS(`{"wants":"beer"}`))
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(5 * 1e9):
		t.Fatal("timeout")
	}
}

func TestEventConditionBindings(t *testing.T) {
	ctx, loc := TestingLocation(t)
	c := make(chan interface{})
	ctx.AddValue("out", c)

	{
		id, err := loc.AddRule(ctx, "r1", mapJS(`
{"when":{"pattern":{"wants":"?x"}},
 "condition":{"code":"console.log('needs ' + event.needs); event.needs == Env.bindings['x'];"},
 "action":{"code":"console.log(JSON.stringify(event)); console.log(x); Env.out('somebody wants and needs ' + x)"}}
`))
		if err != nil {
			t.Fatal(err)
		}
		if id != "r1" {
			t.Fatal(fmt.Errorf("wrong id: '%s'", id))
		}
	}

	go func() {
		_, err := loc.ProcessEvent(ctx, mapJS(`{"wants":"beer","needs":"beer"}`))
		if err != nil {
			t.Fatal(err)
		}
	}()

	select {
	case heard := <-c:
		if heard != "somebody wants and needs beer" {
			t.Fatalf("unexpected message '%s'", heard)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
