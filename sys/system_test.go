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

package sys

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/cron"
)

func JSON(x interface{}) string {
	bs, err := json.MarshalIndent(&x, "  ", "  ")
	if err != nil {
		panic(err)
	}
	return string(bs)
}

func TestReplaceFact(t *testing.T) {

	sys, ctx := ExampleSystem("TestSystem")
	defer sys.Close(ctx)

	location := "there"
	sys.ClearLocation(ctx, location)

	id, err := sys.AddFact(ctx, location, "whatIlike", `{"likes":"chips"}`)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("# AddFact %s\n", id)

	sr, err := sys.SearchFacts(ctx, location, `{"likes":"?x"}`, true)
	fmt.Printf("# SearchFacts %v %v\n", *sr, err)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := sr.Found[0].Bindingss[0]["?x"]
	if 1 != len(sr.Found) {
		t.Fatal("Expected one set of bindings")
	}
	if x != "chips" {
		t.Fatalf("Expected chips but got %s", x)
	}

	id, err = sys.AddFact(ctx, location, "whatIlike", `{"likes":"tacos"}`)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("# AddFact %s\n", id)

	sr, err = sys.SearchFacts(ctx, location, `{"likes":"?x"}`, true)
	fmt.Printf("# SearchFacts %v %v\n", *sr, err)
	if err != nil {
		t.Fatal(err)
	}
	if 1 != len(sr.Found) {
		t.Fatal("Expected one set of bindings")
	}
	x, _ = sr.Found[0].Bindingss[0]["?x"]
	if x != "tacos" {
		t.Fatalf("Expected tacos but got %s", x)
	}

}

func TestDisableRule(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")
	defer sys.Close(ctx)
	location := "there"
	sys.ClearLocation(ctx, location)

	c := make(chan interface{})
	timeout := make(chan bool)
	ctx.AddValue("out", c)

	if _, err := sys.AddFact(ctx, location, "", `{"likes":"chips"}`); err != nil {
		t.Fatal(err)
	}

	rid, err := sys.AddRule(ctx, location, "", `
{"when":{"pattern":{"arrived":"?who"}},
 "condition":{"pattern":{"likes":"?x"}},
 "action":{"code":"Env.out(who)"}}
`)
	if err != nil {
		t.Fatal(err)
	}

	timer := func() {
		time.Sleep(5 * time.Second)
		timeout <- true
	}

	wait := func(expect bool) {
		select {
		case got := <-c:
			fmt.Printf("Got: %s\n", got)
			if !expect {
				t.Fatal("Unexpected receive")
			}
			// Eat the timeout
			<-timeout
		case <-timeout:
			fmt.Printf("Timed out\n")
			if expect {
				t.Fatal("Unexpected timeout")
			}
		}
	}

	send := func() {
		ep, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`)
		// Don't use fmt.Printf() unless you want a data race (on os.Stdout).
		log.Printf("# ProcessEvent %v err=%v\n", *ep, err)
	}

	go timer()
	go send()
	// We expect the rule to fire.
	wait(true)

	// Disable the rule.
	if err = sys.EnableRule(ctx, location, rid, false); err != nil {
		t.Fatal(err)
	}

	go timer()
	go send()
	// We do not expect the rule to fire.
	wait(false)

	// Enable the rule.
	if err = sys.EnableRule(ctx, location, rid, true); err != nil {
		t.Error(err)
	}

	go timer()
	go send()
	// We expect the rule to fire.
	wait(true)
}

func TestRuleId(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")

	defer sys.Close(ctx)
	location := "there"

	sys.ClearLocation(ctx, location)

	id, err := sys.AddFact(ctx, location, "", `{"likes":"chips"}`)
	fmt.Printf("# AddFact %s %v\n", id, err)

	id, err = sys.AddRule(ctx, location, "", `
{"when":{"pattern":{"arrived":"?who"}},
 "condition":{"pattern":{"likes":"?x"}},
 "action":{"code":["console.log('Buy some ' + x + ' for ' + who + '.');",
                   "console.log('My rule ID: ' + ruleId);",
                   "console.log('Example service ' + Env.App.Services.example);"]}}
`)
	fmt.Printf("# AddRule %v %v\n", id, err)

	fr, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`)
	if err != nil {
		t.Fatal(err)
	}
	if fr == nil {
		t.Fatal("nil fr")
	}
	fmt.Printf("# ProcessEvent %v\n", *fr)
}

func TestRuleOverwrite(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")
	// ctx.LogAccumulator = NewAccumulator(10000)
	// ctx.LogAccumulatorLevel = ANYINFO
	// ctx.Verbosity = EVERYTHING
	defer sys.Close(ctx)
	location := "there"

	sys.ClearLocation(ctx, location)

	id, err := sys.AddRule(ctx, location, "foo", `
{"when":{"pattern":{"arrived":"?who"}},
 "action":{"code":["who + ' arrived'"]}}
`)
	fmt.Printf("# AddRule %v %v\n", id, err)
	if err != nil {
		t.Fatal(err)
	}

	ep, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`)
	if err != nil {
		t.Fatal(err)
	}

	check := func(ep *FindRules, want string) {
		results := ep.Values
		got, ok := results[0].(string)
		if !ok {
			t.Fatalf("didn't want a %T (%#v)", got, got)
		}
		if got != want {
			t.Fatalf("wanted '%s', not '%s' (%#v)", want, got, results)
		}
	}

	check(ep, "homer arrived")

	id, err = sys.AddRule(ctx, location, "foo", `
{"when":{"pattern":{"arrived":"?who"}},
 "action":{"code":["who + ' is home'"]}}
`)
	fmt.Printf("# AddRule %v %v\n", id, err)
	if err != nil {
		t.Fatal(err)
	}

	ep, err = sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`)
	if err != nil {
		t.Fatal(err)
	}

	check(ep, "homer is home")
}

func TestSystemWriteKey(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")
	defer sys.Close(ctx)
	location := "there"

	kid, err := sys.AddFact(ctx, location, "", `{"!writeKey":"dogfish"}`)
	if err != nil {
		t.Error(err)
	}
	t.Logf("writeKey fact ID: %s", kid)

	_, err = sys.AddFact(ctx, location, "", `{"wants":"chips"}`)
	if nil == err {
		t.Error("AddFact should have failed due to WriteKey")
	}
	ctx.WriteKey = "dogfish"
	_, err = sys.AddFact(ctx, location, "", `{"wants":"chips"}`)
	if nil != err {
		t.Error("AddFact should have succeeded with key")
	}

	if _, err := sys.RemFact(ctx, location, kid); err != nil {
		t.Error(err)
	}

	ctx.WriteKey = ""
	_, err = sys.AddFact(ctx, location, "", `{"wants":"chips"}`)
	if nil != err {
		t.Error("AddFact should have succeeded")
	}
}

func TestSystemBasic(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")
	defer sys.Close(ctx)
	location := "there"
	noLocation := "not exist!"

	sys.ClearLocation(ctx, location)

	for _, location := range sys.GetCachedLocations(ctx) {
		fmt.Printf("# Clearing %v\n", location)
		if err := sys.ClearLocation(ctx, location); err != nil {
			t.Fatal(err)
		}
	}

	id, err := sys.AddFact(ctx, location, "", `{"likes":"chips"}`)
	fmt.Printf("# AddFact %s %v\n", id, err)
	id, err = sys.AddFact(ctx, location, "", `{"likes":"beer"}`)
	fmt.Printf("# AddFact %s %v\n", id, err)
	id, err = sys.AddFact(ctx, location, "", `{"wants":"tacos"}`)
	fmt.Printf("# AddFact %s %v\n", id, err)
	id, err = sys.AddFact(ctx, location, "", `{"wants":"tacos"}`)
	fmt.Printf("# AddFact %s %v\n", id, err)

	n, err := sys.GetSize(ctx, location)
	if nil != err {
		t.Error(err)
	}
	fmt.Printf("# Size %d\n", n)

	sr, err := sys.SearchFacts(ctx, location, `{"likes":"?x"}`, true)
	fmt.Printf("# SearchFacts %v %v\n", *sr, err)

	id, err = sys.RemFact(ctx, location, id)
	fmt.Printf("# RemFact %v %v\n", id, err)

	sr, err = sys.SearchFacts(ctx, location, `{"likes":"?x"}`, true)
	fmt.Printf("# SearchFacts %v %v\n", *sr, err)

	id, err = sys.AddRule(ctx, location, "", `
{"when":{"pattern":{"arrived":"?who"}},
 "condition":{"pattern":{"likes":"?x"}},
 "action":{"code":["console.log('Buy some ' + x + ' for ' + who + '.')",
                   "console.log('Example service ' + Env.App.Services.example)"]}}
`)
	fmt.Printf("# AddRule %v %v\n", id, err)

	ep, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`)
	fmt.Printf("# ProcessEvent %v %v\n", *ep, err)

	// Deliberately do something wrong.
	id, err = sys.AddFact(ctx, location, "", `{"wants":badjson}`)
	fmt.Printf("# Bad AddFact %s %v\n", id, err)

	// Named rule
	id, err = sys.AddRule(ctx, location, "goodrule", `
{"when":{"pattern":{"arrived":"?who"}},
 "condition":{"pattern":{"likes":"?x"}},
 "action":{"code":["console.log('Buy some ' + x + ' for ' + who + '.')",
                   "console.log('Example service ' + Env.App.Services.example)"]}}
`)
	fmt.Printf("# AddRule (named) %v %v\n", id, err)

	// Get that rule

	rule, err := sys.GetRule(ctx, location, id)
	fmt.Printf("# GetRule (named) %v %v\n", rule, err)

	stats, err := sys.GetLocationStats(ctx, location)
	fmt.Printf("LocationStats %v %v\n", *stats, err)

	for _, location := range sys.GetCachedLocations(ctx) {
		err := sys.ClearLocation(ctx, location)
		fmt.Printf("# Cleared %s %v\n", location, err)
	}

	stats, err = sys.GetStats(ctx)
	fmt.Printf("SystemStats %v %v\n", *stats, err)

	// Temporary: Do not run the tests that verify the
	// non-existence of a location.  ToDo: Turn these back on
	// soon.
	doNotCreateLocationTests := false

	if doNotCreateLocationTests {
		_, err = sys.SearchFacts(ctx, noLocation, `{"likes":"?x"}`, true)
		if _, ok := err.(*NotFoundError); !ok {
			t.Error(noLocation + " does not exist!")
		}

		_, err = sys.RemFact(ctx, noLocation, id)
		if _, ok := err.(*NotFoundError); !ok {
			t.Error(noLocation + " does not exist!")
		}

		_, err = sys.ProcessEvent(ctx, noLocation, `{"arrived":"homer"}`)
		if _, ok := err.(*NotFoundError); !ok {
			t.Error(noLocation + " does not exist!")
		}

		_, err = sys.GetLocationStats(ctx, noLocation)
		if _, ok := err.(*NotFoundError); !ok {
			t.Error(noLocation + " does not exist!")
		}
	}
}

func benchmarkSystemSimple(b *testing.B, checking bool, unindexed bool) {
	sys, ctx := BenchSystem("BenchmarkSystemSimple")
	defer sys.Close(ctx)

	sys.config.CheckExistence = checking
	sys.config.UnindexedState = unindexed

	location := "there"

	if checking {
		if _, err := sys.CreateLocation(ctx, location); err != nil {
			b.Fatal(err)
		}
	}

	sys.ClearLocation(ctx, location)

	sys.AddFact(ctx, location, "", `{"likes":"chips"}`)
	sys.AddFact(ctx, location, "", `{"likes":"beer"}`)
	sys.AddFact(ctx, location, "", `{"wants":"tacos"}`)
	sys.AddFact(ctx, location, "", `{"wants":"tacos"}`)
	for i := 0; i < 50; i++ {
		sys.AddFact(ctx, location, "", fmt.Sprintf(`{"ignore_%d":"this_%d"}`, i, i))
	}

	sys.AddRule(ctx, location, "", `
{"when":{"pattern":{"arrived":"?who"}},
 "condition":{"pattern":{"likes":"?x"}},
 "action":{"code":["// console.log('Buy some ' + x + ' for ' + who + '.'); \n",
                   "var fired = Env.Context.Props['fired'];",
                   "if (!fired) { fired = 0 }",
                   "fired++;",
                   "Env.Context.Props['fired'] = fired;"]}}
`)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer"}`); nil != err {
			b.Error(err)
		}
	}
	stats, _ := sys.GetStats(ctx)
	b.Logf("b.N %d Stats %v\n", b.N, *stats)
}

func BenchmarkSystemSimpleIndexedChecking(b *testing.B) {
	benchmarkSystemSimple(b, true, false)
}

func BenchmarkSystemSimpleIndexedNotChecking(b *testing.B) {
	benchmarkSystemSimple(b, false, false)
}

func BenchmarkSystemSimpleLinearChecking(b *testing.B) {
	benchmarkSystemSimple(b, true, true)
}

func BenchmarkSystemSimpleLinearNotChecking(b *testing.B) {
	benchmarkSystemSimple(b, false, true)
}

func BenchmarkSystemMoreFactsAndRules(b *testing.B) {
	sys, ctx := BenchSystem("BenchmarkSystemMoreFactsAndRules")
	location := "here"

	sys.ClearLocation(ctx, location)

	for i := 0; i < 100; i++ {
		if _, err := sys.AddFact(ctx, location, "",
			fmt.Sprintf(`{"person":"homer", "hates":"diet%d", "at":"place%d"}`, i, i%10)); nil != err {
			b.Error(err)
		}
	}
	sys.AddFact(ctx, location, "",
		`{"person":"homer", "likes":"chips", "at":"place0"}`)

	for i := 0; i < 100; i++ {
		sys.AddRule(ctx, location, "", fmt.Sprintf(`
{"when":{"pattern":{"arrived":"?who","at":"?place"}},
 "condition":{"pattern":{"person":"?who","likes":"?x","at":"?place"}},
 "action":{"code":["// console.log('Buy some ' + x + ' for ' + who + ' at ' + place + '.');\n",
                   "var fired = Env.Context.Props['fired'];",
                   "if (!fired) { fired = 0 }",
                   "fired++;",
                   "Env.Context.Props['fired'] = fired;"]}}
`))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := sys.ProcessEvent(ctx, location, `{"arrived":"homer","at":"place0"}`); nil != err {
			b.Error(err)
		}
	}
	stats, _ := sys.GetStats(ctx)
	b.Logf("b.N %d Stats %v\n", b.N, *stats)
}

func TestSystemUnmarshalDuration(t *testing.T) {
	var d Duration
	d.UnmarshalJSON([]byte("1000"))
}

func TestSystemRemRulesMisc(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystemRemRulesMisc")
	defer sys.Close(ctx)
	location := "here"

	sys.ClearLocation(ctx, location)

	id, err := sys.AddRule(ctx, location, "", fmt.Sprintf(`
{"when":{"pattern":{"arrived":"?who","at":"?place"}},
 "condition":{"pattern":{"person":"?who","likes":"?x","at":"?place"}},
 "action":{"code":["console.log('Buy some ' + x + ' for ' + who + ' at ' + place + '.');"]}}
`))

	if nil != err {
		t.Error(err)
	}
	fmt.Printf("# AddRule id %s\n", id)

	sr, err := sys.SearchRules(ctx, location, `{"arrived":"homer","at":"home"}`, true)
	if len(sr) != 1 {
		t.Errorf("Expected to find 1 rule but found %d", len(sr))
	}
	_, found := sr[id]
	if !found {
		t.Errorf("Expected to find rule with id %s but didn't", id)
	}

	rules, err := sys.ListRules(ctx, location, true)
	if nil != err {
		t.Error(err)
	}
	if len(rules) != 1 {
		for _, rule := range rules {
			fmt.Printf("# ListRules rule %v\n", rule)
		}
		t.Errorf("Expected to list one rule but got %d", len(rules))

	}

	_, err = sys.RemRule(ctx, location, id)
	if nil != err {
		t.Error(err)
	}

	sr, err = sys.SearchRules(ctx, location, `{"arrived":"homer","at":"home"}`, true)
	if len(sr) != 0 {
		t.Errorf("Expected to find no rules but found %d", len(sr))
	}

}

func TestBadStatelessWithExternalCron(t *testing.T) {
	name := "test"

	// First we try to do things right.

	ctx := TestContext(name)
	cont := ExampleSystemControl()
	// Cache locations Forever, which will make it okay to use the
	// in-memory cron.
	cont.LocationTTL = Forever
	conf := ExampleConfig()
	cr, _ := cron.NewCron(nil, time.Second, "intcron", 1000000)
	ic := &cron.InternalCron{
		Cron: cr,
	}

	sys, err := NewSystem(ctx, *conf, *cont, ic)
	if err != nil {
		t.Fatal(err)
	}
	sys.storage, _ = NewNoStorage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = sys.ClearLocation(ctx, name)
	if err != nil {
		t.Fatal(err)
	}

	// Now we do things wrong.  Hope to see a panic and recover.

	waiter := sync.WaitGroup{}
	waiter.Add(1)
	panicked := false
	go func() {
		waiter.Wait()
		if !panicked {
			t.Fatal("never panicked")
		}
	}()

	cont = ExampleSystemControl()
	// Never cache locations, which should mean that we must use a
	// persistent (external) cron.
	cont.LocationTTL = Never

	// We expect a panic, so grab that panic in order to check
	// that we got it.
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered %v\n", r)
			if r != "failed" {
				panicked = true
			}
			waiter.Done()
		}
	}()

	sys, err = NewSystem(ctx, *conf, *cont, ic)
	sys.storage, _ = NewNoStorage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = sys.ClearLocation(ctx, name)
	if err != nil {
		t.Fatal(err)
	}

	panic("failed")
}

func TestSystemIssue303(t *testing.T) {
	sys, ctx := ExampleSystem("TestSystem")
	defer sys.Close(ctx)
	location := "there"
	sys.ClearLocation(ctx, location)

	timestamp := "1439476057719"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("request URI %s", r.RequestURI)
		bs, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		log.Printf("body %s", bs)
		fmt.Fprintf(w, "%s", r.RequestURI)

		js := string(bs)
		if strings.Index(js, timestamp) < 0 {
			t.Fatal("didn't see the right timestamp")
		}
	}))
	defer server.Close()

	fmt.Printf("# test server %v\n", server.URL)

	rule := fmt.Sprintf(`
{
  "when": {
      "pattern": {"name": "AlarmInProgress", "timestamp": "?timestamp"}
  },
  "action": {
    "endpoint": "%s",
    "code": {
      "data": {
          "timestamp": "?timestamp"
      }
    }
  }
}
`, server.URL)

	id, err := sys.AddRule(ctx, location, "", rule)
	fmt.Printf("# AddRule %v %v\n", id, err)

	event := fmt.Sprintf(`
{
   "name": "AlarmInProgress",
   "timestamp": %s
}
`, timestamp)

	ep, err := sys.ProcessEvent(ctx, location, event)

	fmt.Printf("# ProcessEvent %v %v\n", *ep, err)
}
