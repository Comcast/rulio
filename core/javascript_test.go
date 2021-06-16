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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)


// tests the 'out' javascript callable
func TestJavascriptCommunication(t *testing.T) {
	ctx := NewContext("test")
	c := make(chan interface{})
	defer close(c)
	ctx.AddProp("out", c)

	wg := sync.WaitGroup{}
	go func() {
		val :=<- c
		s, ok := val.(string)
		if !ok {
			t.Errorf("expected type from channel communication to be string. value from channel was : %#v", val)
		}

		if s != "foo" {
			t.Errorf("expected string 'foo' to be sent on channel. actual was: %s", s)
		}
		wg.Done()
	}()

	wg.Add(1)
	_, err := RunJavascript(ctx, nil, nil, "Env.out('foo')")
	if err != nil {
		t.Error(err)
	}
	wg.Wait()
}

// scoped runtimes without an explicit return will return nil. unscoped runtimes continue to work as expected.
func TestRunJavascriptRunoffResult(t *testing.T) {
	ctx := NewContext("TestJavascriptLocation")

	bs := make(Bindings)
	bs["x"] = 2
	SystemParameters.ScopedJavascriptRuntimes = true

	// reused/scoped runtimes should still implicitly return a value
	val, err := RunJavascript(ctx, &bs,  nil, `x + 2`)
	if err != nil {
		t.Error(err)
	}

	if v, ok := val.(float64); ok {
		if v != 4 {
			t.Errorf("expected 2+2=4, got: %f", v)
		}
	} else {
		t.Errorf("expected return value to be a float, got: %#v", val)
	}

	SystemParameters.ScopedJavascriptRuntimes = false
	val, err = RunJavascript(ctx, &bs,  nil, `x + 2`)
	if err != nil {
		t.Error(err)
	}

	if v, ok := val.(float64); ok {
		if v != 4 {
			t.Errorf("expected 2+2=4, got: %f", v)
		}
	} else {
		t.Errorf("expected return value to be a float, got: %#v", val)
	}
}

// verify that javascript won't mutate the bindings pointer
func TestJavascriptBindingMutation(t *testing.T) {
	bs := make(Bindings)
	var val *int
	val = new(int)
	*val = 2
	bs["x"] = val
	SystemParameters.ScopedJavascriptRuntimes = true
	_, err := RunJavascript(nil, &bs, nil, "x=x+1")
	if err != nil {
		t.Error(err)
	}
	if *bs["x"].(*int) != 2 {
		t.Errorf("Expected bindings not to be changed from javascript execution")
	}

	SystemParameters.ScopedJavascriptRuntimes = false
	_, err = RunJavascript(nil, &bs, nil, "x=x+1")
	if err != nil {
		t.Error(err)
	}
	if *bs["x"].(*int) != 2 {
		t.Errorf("Expected bindings not to be changed from javascript execution")
	}
}

// verify that state doesn't leak between executions
//note the `var` declaration used.
func TestJavascriptStateMutation(t *testing.T) {
	ctx := NewContext("test")
	c := make(chan interface{})
	defer close(c)
	ctx.AddProp("out", c)

	go func() {
		for val := range c {
			t.Errorf("was not expecting any value to be sent. '%#v' was unexpected", val)
		}
	}()

	bs := make(Bindings)
	bs["x"] = 2
	SystemParameters.ScopedJavascriptRuntimes = true
	for i := 0; i < 10; i++ {
		_, err := RunJavascript(ctx, &bs, nil, "if (typeof y != 'undefined') { Env.out('y is not undefined') }; var y = 1")
		if err != nil {
			t.Error(err)
		}
	}

	// if the variable is declared without var, state will leak out of the scope for future executions
	_, err := RunJavascript(ctx, &bs, nil, "y = 1")
	if err != nil {
		t.Error(err)
	}
	// there's a chance that the sync.pool will reap a member and create a new runtime in this execution.  We don't have control over this.
	// in practice, this has not been observed due to these tests being sequential
	_, err = RunJavascript(ctx, &bs, nil, "if (typeof y == 'undefined') { Env.out('y is undefined, expected to assert state leakage') }")
	if err != nil {
		t.Error(err)
	}

	SystemParameters.ScopedJavascriptRuntimes = false
	for i := 0; i < 10; i++ {
		_, err := RunJavascript(ctx, &bs, nil, "if (typeof y != 'undefined') { Env.out('y is not undefined') }; var y = 1")
		if err != nil {
			t.Error(err)
		}
	}

}

func TestJavascript(t *testing.T) {
	bs := make(Bindings)
	bs["x"] = 2
	_, err := RunJavascript(nil, &bs, nil, "x+1")
	if err != nil {
		t.Error(err)
	}
}

func TestJavascriptError(t *testing.T) {
	bs := make(Bindings)
	bs["x"] = 2
	_, err := RunJavascript(nil, &bs, nil, `
try {
  console.log(Env.get('badurl'));
} catch (err) {
  console.log("caught " + err)
}`)
	if err != nil {
		t.Error(err)
	}
}

func DontTestJavascriptGet(t *testing.T) {
	bs := make(Bindings)
	v, err := RunJavascript(nil, &bs, nil, `
json = Env.get("http://api.openweathermap.org/data/2.5/weather?units=metric&q=Austin,Texas");
console.log(json)
weather = JSON.parse(json);
weather["main"]["temp"]
`)
	fmt.Printf("Weather: %v (%T) %v\n", v, v, err)
	if err != nil {
		t.Error(err)
	}
}

func BenchmarkJavascriptAddition(b *testing.B) {
	ctx := BenchContext("BenchmarkJavascriptAddition")

	for i := 0; i < b.N; i++ {
		_, err := RunJavascript(ctx, nil, nil, "1+2+3+4")
		if err != nil {
			b.Errorf("Error %v", err)
		}
	}
}

func BenchmarkJavascriptNoOp(b *testing.B) {
	ctx := BenchContext("BenchmarkJavascriptNoop")

	for i := 0; i < b.N; i++ {
		_, err := RunJavascript(ctx, nil, nil, "1")
		if err != nil {
			b.Errorf("Error %v", err)
		}
	}
	// uncomment for a noisier benchmark, but gives greater insight to leaks
	//mem := runtime.MemStats{}
	//runtime.ReadMemStats(&mem)
	//fmt.Printf("current goroutines: %d\n", runtime.NumGoroutine())
	//fmt.Printf("memory statistics: %+v\n", mem)
}

func BenchmarkJavascriptLoop(b *testing.B) {
	ctx := BenchContext("BenchmarkJavascriptLoop")

	ops := 1000
	code := fmt.Sprintf("var acc = 0; for (i = 0; i < %d; i++) { acc++ }", ops)
	for i := 0; i < b.N; i++ {
		_, err := RunJavascript(ctx, nil, nil, code)
		if err != nil {
			b.Errorf("Error %v", err)
		}
	}
}

func ExampleRunJavascript() {
	c := TestContext("ExampleRunJavascript")
	bs := Bindings(map[string]interface{}{"a": 1, "b": 2})
	RunJavascript(c, &bs, nil, "console.log(a+b)")
	// Output:
	// 3
}

func TestJavascriptSecsFromNow(t *testing.T) {
	bs := make(Bindings)
	bs["x"] = "2015-04-14T19:40:59.513Z"
	secs, err := RunJavascript(nil, &bs, nil, "Env.secsFromNow(x)")
	if err != nil {
		t.Error(err)
	}
	n, ok := secs.(float64)
	t.Logf("secs %T %#v\n", secs, secs)
	if !ok {
		t.Fatal("not a float64")
	}
	if n < 2000 {
		t.Fatal("wrong answer")
	}
}

func TestJavascriptBindings(t *testing.T) {
	bs := make(Bindings)
	bar := "bar"
	bs["foo"] = bar
	got, err := RunJavascript(nil, &bs, nil, "Env.bindings['foo']")
	if err != nil {
		t.Error(err)
	}

	if bar != got {
		t.Errorf("got unexpected '%s'", got)
	}
}

func TestJavascriptOut(t *testing.T) {
	ctx := NewContext("TestJavascriptOut")
	out := make(chan interface{})
	ctx.AddValue("out", out)

	go func() {
		bs := make(Bindings)
		_, err := RunJavascript(ctx, &bs, nil, "Env.out('foo')")
		if err != nil {
			t.Error(err)
		}
	}()

	got := <-out

	if got != "foo" {
		t.Errorf("got unexpected '%s'", got)
	}
}

func TestJavascriptMatch(t *testing.T) {
	bs := make(Bindings)
	got, err := RunJavascript(nil, &bs, nil, "Env.match({likes:'?what'},{likes:'chips'})")
	if err != nil {
		t.Error(err)
	}
	switch bss := got.(type) {
	case []Bindings:
		if len(bss) == 1 {
			what, have := bss[0]["?what"]
			if have && what == "chips" {
				return // Happy.
			}
		}
	}
	t.Fatalf("unexpected %#v", got)
}

func TestJavascriptLocation(t *testing.T) {
	name := "test"
	ctx := NewContext("TestJavascriptLocation")

	loc, err := NewLocation(ctx, name, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx.SetLoc(loc)

	bs := make(Bindings)
	got, err := RunJavascript(ctx, &bs, nil, "Env.Location")
	if err != nil {
		t.Error(err)
	}
	switch loc := got.(type) {
	case string:
		if loc == name {
			return // Happy.
		}
	}
	t.Fatalf("unexpected %#v", got)
}

func TestJavascriptVersion(t *testing.T) {
	bs := make(Bindings)
	happy, err := RunJavascript(nil, &bs, nil, "v = Env.Versions[0]; v == Envs[v].Version")
	if err != nil {
		t.Error(err)
	}
	switch b := happy.(type) {
	case bool:
		if b {
			return
		}
	}

	t.Fatalf("unexpected %#v", happy)
}

func TestCommandSpec(t *testing.T) {
	cs := CommandSpec{
		Path: "/bin/ls",
		Args: []string{"/tmp"},
	}
	if err := cs.Exec(nil); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("stdout %s\n", cs.Stdout)
	fmt.Printf("stderr %s\n", cs.Stderr)
}

func TestJavascriptExec(t *testing.T) {
	bs := make(Bindings)
	{
		code := `Env.exec({"path":"/bin/ls", "args":["/tmp"]})`
		x, err := RunJavascript(nil, &bs, nil, code)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("x %#v", x)
	}

	{
		code := `Env.exec({"path":"/usr/bin/wc", "args":["-l"],"stdin":"chips\ntacos\nqueso\n"})`
		x, err := RunJavascript(nil, &bs, nil, code)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("x %#v", x)
	}

	{
		code := `Env.exec({"path":"/bin/bash", "args":["-c", "echo LIKES=$LIKES"], "env":{"LIKES":"chips"}})`
		x, err := RunJavascript(nil, &bs, nil, code)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("x %#v", x)
	}

	{
		code := `Env.bash("echo LIKES=$LIKES", {"env":{"LIKES":"chips"}})`
		x, err := RunJavascript(nil, &bs, nil, code)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("x %#v", x)
	}

	{
		code := `Env.bash("test ! -f /tmp/does/not/exist").Success`
		x, err := RunJavascript(nil, &bs, nil, code)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("x %#v", x)
	}

}

func TestJavascriptAppUpdate(t *testing.T) {
	ctx := TestContext("Test")
	ctx.App = &BindingApp{}
	got, err := RunJavascript(ctx, nil, nil, "Napoleon")
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := got.(string); !ok || s != "Dynamite" {
		t.Fatalf("didn't expect %#v", got)
	}
}

func TestJavascriptHttpx(t *testing.T) {
	faceUp := "Can't seem to face up to the facts"
	// I'm tense and nervous; can't relax.
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(faceUp))
		// I can't sleep 'cause my bed's on fire.
		// Don't touch me; I'm a real live wire.
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	code := fmt.Sprintf(`Env.httpx({URI: "%s/foo"}).Body`, server.URL)
	bs := make(Bindings)
	x, err := RunJavascript(nil, &bs, nil, code)
	if err != nil {
		t.Error(err)
	}

	s, is := x.(string)
	if !is {
		t.Fatalf("%#v isn't a %T", x, s)
	}

	if s != faceUp {
		t.Fatalf("'%s' != '%s'", s, faceUp)
	}

}

func TestJavascriptProcessEventNil(t *testing.T) {
	name := "test"
	ctx := NewContext("TestJavascriptLocation")

	loc, err := NewLocation(ctx, name, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx.SetLoc(loc)

	bs := make(Bindings)
	if _, err := RunJavascript(ctx, &bs, nil, `Env.ProcessEvent({})`); err != nil {
		t.Error(err)
	}
}
