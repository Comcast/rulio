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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// RuleTest loads the given facts and rule.  Then submits the given events.
// If 'ctx' is nil, creates a 'SimpleSystem' and clears it.
func RuleTest(ctx *Context, location string, rule string, facts []string, events []string) error {

	if ctx == nil {
		ctx = NewContext("test")
	}

	loc, err := NewLocation(ctx, location, nil, nil)
	if err != nil {
		return err
	}

	if err = loc.Clear(ctx); err != nil {
		return err
	}

	_, err = loc.AddRule(ctx, "", mapJS(rule))
	if err != nil {
		return err
	}

	for _, fact := range facts {
		if _, err := loc.AddFact(ctx, "", mapJS(fact)); err != nil {
			return err
		}
	}

	for _, event := range events {
		loc.ProcessEvent(ctx, mapJS(event))
	}

	return nil
}

func ExampleLocation_ProcessEvent_basic() {
	previous := SystemParameters.IdInjectionTime
	SystemParameters.IdInjectionTime = InjectIdNever
	// Will cause much confusion and trouble with concurrent testing.
	defer func() {
		SystemParameters.IdInjectionTime = previous
	}()

	// Keep the aeroplane in such an attitude that the air
	// pressure is directly in the aviator's face.
	//
	//  --Horatio C. Barber, 1916

	RuleTest(TestContext("ExampleRuleTest1"),
		"TestRules1",
		`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"console.log(\"serve \" + what + \" at \" + there);"}]}
`,
		[]string{`{"likes":"tacos"}`},
		[]string{`{"at":"home"}`})

	// Output:
	// serve tacos at home
}

func TestingLocation(t *testing.T) (*Context, *Location) {
	return NamedTestingLocation(t, "test")
}

func NamedTestingLocation(t *testing.T, name string) (*Context, *Location) {
	ctx := BenchContext(name)

	store, _ := NewNoStorage(ctx)
	state, _ := NewIndexedState(ctx, name, store)
	loc, err := NewLocation(ctx, name, state, nil)

	if err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatal(err)
	}

	loc.SetControl(&Control{MaxFacts: 10000})

	if err = loc.Clear(ctx); err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatal(err)
	}
	ctx.SetLoc(loc)

	return ctx, loc
}

func chanGet(c chan interface{}, wait time.Duration, t *testing.T) (interface{}, error) {
	timeout := time.NewTimer(wait).C
	select {
	case x := <-c:
		return x, nil
	case <-timeout:
		err := errors.New("timeout")
		if t != nil {
			t.Fatal(err)
		} else {
			return nil, err
		}
	}
	return nil, nil
}

func ExampleLocation_ProcessEvent_out() {
	previous := SystemParameters.IdInjectionTime
	SystemParameters.IdInjectionTime = InjectIdNever
	// Will cause much confusion and trouble with concurrent testing.
	defer func() {
		SystemParameters.IdInjectionTime = previous
	}()

	c := make(chan interface{})

	ctx := TestContext("test")
	ctx.AddValue("out", c)

	go RuleTest(ctx,
		"TestRules1",
		`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`,
		[]string{`{"likes":"tacos"}`},
		[]string{`{"at":"home"}`})

	x, _ := chanGet(c, 6*time.Second, nil)
	fmt.Printf("Got: %#v\n", x)

	// Output:
	// Got: "serve tacos at home"
}

func BenchmarkFactEventRule(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c := make(chan interface{})
		ctx := BenchContext("test")
		ctx.AddValue("out", c)
		go RuleTest(ctx,
			fmt.Sprintf("TestRules_%d", i),
			`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`,
			[]string{`{"likes":"tacos"}`},
			[]string{`{"at":"home"}`})

		chanGet(c, 3*time.Second, nil)
	}
}

func BenchFactsEventsRules(b *testing.B, rules []string, facts []string, events []string,
	actionsPerEvent int,
	resetTimerBeforeEvents bool) {
	// b.SetParallelism(12)

	ctx, loc := TestingLocation(nil)

	for _, rule := range rules {
		_, err := loc.AddRule(ctx, "", mapJS(rule))
		if err != nil {
			b.Errorf("addRule: %v", err)
			return
		}
	}

	for _, fact := range facts {
		_, err := loc.AddFact(ctx, "", mapJS(fact))
		if err != nil {
			b.Errorf("addFact: %v", err)
			return
		}
	}

	es := make([]Map, len(events))
	for i, e := range events {
		es[i] = mapJS(e)
	}

	if resetTimerBeforeEvents {
		b.ResetTimer()
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := BenchContext("BenchmarkFactEventRule")
			c := make(chan interface{})
			ctx.AddValue("out", c)

			go func() {
				for _, event := range es {
					loc.ProcessEvent(ctx, event)
				}
			}()

			expect := actionsPerEvent * len(es)
			for 0 < expect {
				chanGet(c, 3*time.Second, nil)
				expect--
			}
		}

	})
}

func BenchmarkFactRuleEvents10(b *testing.B) {

	// Make 10 events.
	places := []string{
		"home", "work", "car", "hotel", "curb",
		"castle", "roof", "shed", "hut", "island"}
	events := make([]string, 0, len(places))
	for _, place := range places {
		events = append(events, fmt.Sprintf(`{"at":"%s"}`, place))
	}

	BenchFactsEventsRules(b,
		[]string{`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`},
		[]string{`{"likes":"tacos"}`},
		events,
		1,
		false)

}

func BenchmarkFacts10RuleEvents10(b *testing.B) {

	// Make 10 events.
	places := []string{
		"home", "work", "car", "hotel", "curb",
		"castle", "roof", "shed", "hut", "island"}
	events := make([]string, 0, len(places))
	for _, place := range places {
		events = append(events, fmt.Sprintf(`{"at":"%s"}`, place))
	}

	// Make 10 facts.
	things := []string{
		"tacos", "chips", "beer", "nachos", "salsa",
		"pie", "ice cream", "Yamazaki", "Moscow mule", "burger"}
	facts := make([]string, 0, len(things))
	for _, thing := range things {
		facts = append(facts, fmt.Sprintf(`{"likes":"%s"}`, thing))
	}

	b.ResetTimer()

	BenchFactsEventsRules(b,
		[]string{`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`},
		facts,
		events,
		10,
		true)

}

func BenchmarkFacts1000RuleEvents10(b *testing.B) {

	// Make 10 events.
	places := []string{
		"home", "work", "car", "hotel", "curb",
		"castle", "roof", "shed", "hut", "island"}
	events := make([]string, 0, len(places))
	for _, place := range places {
		events = append(events, fmt.Sprintf(`{"at":"%s"}`, place))
	}

	// Make 10 facts.
	// And 990 more irrelevant facts.
	things := []string{
		"tacos", "chips", "beer", "nachos", "salsa",
		"pie", "ice cream", "Yamazaki", "Moscow mule", "burger"}
	facts := make([]string, 0, len(things)+990)
	for _, thing := range things {
		facts = append(facts, fmt.Sprintf(`{"likes":"%s"}`, thing))
	}

	for i := 0; i < 990; i++ {
		// https://www.goodreads.com/quotes/6425-i-have-always-wanted-to-write-a-book-that-ended
		facts = append(facts, fmt.Sprintf(`{"hates":"mayonnaise_%d"}`, i))
	}

	b.ResetTimer()

	BenchFactsEventsRules(b,
		[]string{`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`},
		facts,
		events,
		10,
		true)

}

func BenchmarkFacts1000Rules200Events10(b *testing.B) {

	// Make 10 events.
	places := []string{
		"home", "work", "car", "hotel", "curb",
		"castle", "roof", "shed", "hut", "island"}
	events := make([]string, 0, len(places))
	for _, place := range places {
		events = append(events, fmt.Sprintf(`{"at":"%s"}`, place))
	}

	// Make 10 facts and
	things := []string{
		"tacos", "chips", "beer", "nachos", "salsa",
		"pie", "ice cream", "Yamazaki", "Moscow mule", "burger"}
	facts := make([]string, 0, len(things)+990)
	for _, thing := range things {
		facts = append(facts, fmt.Sprintf(`{"likes":"%s"}`, thing))
	}

	// 990 more irrelevant facts.
	for i := 0; i < 990; i++ {
		// https://www.goodreads.com/quotes/6425-i-have-always-wanted-to-write-a-book-that-ended
		facts = append(facts, fmt.Sprintf(`{"hates":"mayonnaise_%d"}`, i))
	}

	// Make 200 rules.
	rules := make([]string, 0, 200)
	// One good one
	rules = append(rules, `
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`)
	// and some irrelevant ones
	for i := 0; i < 99; i++ {
		// Bad 'when'
		rule := fmt.Sprintf(`
{"when":{"pattern":{"ignore_%d":"?there"}},
 "condition":{"pattern":{"likes":"?what"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`, i)
		facts = append(facts, rule)
	}

	// and some more irrelevant ones
	for i := 0; i < 99; i++ {
		// Bad 'condition'
		rule := fmt.Sprintf(`
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"beer_%d"}},
 "actions":[{"code":"Env.out(\"serve \" + what + \" at \" + there);"}]}
`, i)
		facts = append(facts, rule)
	}

	b.ResetTimer()

	BenchFactsEventsRules(b, rules, facts, events, 10, true)

}

func TestRuleFromMapBasic(t *testing.T) {
	ctx := NewContext("TestRuleFromMapBasic")
	js := `
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`
	m := fromJSON(js)

	if _, err := RuleFromMap(ctx, m); err != nil {
		t.Fatal(err)
	}
}

func TestRuleFromMapWithProp(t *testing.T) {
	ctx := NewContext("TestRuleFromMapWithProp")
	js := `
{"when":{"pattern":{"at":"?there"}},
 "props":{"wants":"queso"},
 "condition":{"pattern":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`
	m := fromJSON(js)

	r, err := RuleFromMap(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	queso, have := r.Props["wants"]
	if !have || queso != "queso" {
		t.Fatal("lost what I wanted")
	}
}

func testRuleFromMapBad(t *testing.T, tag string, js string) {
	ctx := NewContext("TestRuleFromMap" + tag)
	m := fromJSON(js)
	if _, err := RuleFromMap(ctx, m); err == nil {
		t.Fatal(err)
	}
}

func TestRuleFromMapNoAction(t *testing.T) {
	testRuleFromMapBad(t, "NoAction", `
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"beer_%d"}}}
`)
}

func TestRuleFromMapNoWhen(t *testing.T) {
	testRuleFromMapBad(t, "NoWhen", `
{"condition":{"pattern":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`)
}

func TestRuleFromMapWhenAndSchedule(t *testing.T) {
	testRuleFromMapBad(t, "WhenAndSchedule", `
{"when":{"pattern":{"at":"?there"}},
 "schedule": "+1s",
 "condition":{"pattern":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`)
}

func TestRuleFromMapBadCondition(t *testing.T) {
	testRuleFromMapBad(t, "BadCondition", `
{"when":{"pattern":{"at":"?there"}},
 "condition":{"homer":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`)
}

func BenchmarkRuleFromMapBasic(b *testing.B) {
	ctx := BenchContext("BenchmarkRuleFromMapBasic")
	js := `
{"when":{"pattern":{"at":"?there"}},
 "condition":{"pattern":{"likes":"beer_%d"}},
 "action":{"code":"console.log('hello')"}}
`
	m := fromJSON(js)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := RuleFromMap(ctx, m); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRuleWithUnboundVar(t *testing.T) {

	handled := make(chan error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := ioutil.ReadAll(r.Body)
		if err != nil {
			handled <- err
			return
		}
		defer r.Body.Close()
		fmt.Printf("handling %s\n", got)
		var m map[string]interface{}
		err = json.Unmarshal(got, &m)
		if err != nil {
			handled <- err
			return
		}
		code, given := m["code"]
		if !given {
			handled <- errors.New("no code")
			return
		}
		js, err := json.Marshal(&code)
		if err != nil {
			handled <- err
			return
		}
		fmt.Printf("code %s\n", js)
		unbound := 0 < strings.Index(string(js), "?instanceName")
		if unbound {
			handled <- errors.New("saw unbound")
			return
		}
		fmt.Fprintln(w, "hello")
		handled <- nil
	}))
	defer server.Close()

	url := server.URL
	fmt.Printf("url %s", url)

	err := RuleTest(TestContext("TestRuleWithUnboundVar"),
		"TestRuleWithUnboundVar1",
		fmt.Sprintf(`
{
    "when": {
        "pattern": {
            "name": "Foo",
            "timestamp": "?timestamp"
        }
    },

    "condition": {
        "or": [
            {
                "and": [
                    {
                        "pattern": {
                            "event": "zone",
                            "time": "?time",
                            "instanceName": "?instanceName"
                        }
                    },
                    {
                        "code": "0 < time"
                    }
                ]
            },
            {
                "code": "true"
            }
        ]
    },

    "action": {
        "endpoint": "%s",
        "code": {
            "location": "20905xxxxxxxxxxxxx15",
            "app": "myapp",
            "type": "FOO",
            "transport": [
                "emo"
             ],
            "params": {
                "topic": "/xhs/tps/20905xxxxxxxxxxxxx15/foo"
             },
            "data": {
                "timestamp": "?timestamp",
                "zone": "?instanceName"
            }
        }
    }
}
`, url),

		[]string{`{"event":"zone","time":0,"instanceName":"tacos"}`},
		[]string{`{"name":"Foo","timestamp":"then"}`})

	if err != nil {
		t.Fatal(err)
	}

	timer := time.NewTicker(5 * time.Second).C
	select {
	case problem := <-handled:
		if problem != nil {
			t.Fatal(problem)
		}
	case <-timer:
		// Victory
	}
}
