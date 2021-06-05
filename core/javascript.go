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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/robertkrimen/otto"
)

var Halt = errors.New("halt")

var slurpClient *http.Client

func init() {
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	c := &http.Client{Transport: t}
	// Variable initializations are run before init() functions.
	c.Timeout = SystemParameters.SlurpTimeout
	slurpClient = c
	ParametersAddHook(func(p *Parameters) error {
		Log(DEBUG, nil, "javascript.hook", "timeout", p.SlurpTimeout)
		// ToDo: Make it thread-safe?
		slurpClient.Timeout = p.SlurpTimeout
		return nil
	})

}

// Slurp gets the contents (string) at a URL.
// Also see CachedSlurp.
func Slurp(ctx *Context, url string) (string, error) {
	timer := NewTimer(ctx, "Slurp")
	defer timer.Stop()

	Log(INFO, ctx, "core.Slurp", "url", url)

	resp, err := slurpClient.Get(url)
	var body []byte
	if err == nil {
		body, err = ioutil.ReadAll(resp.Body)
	}

	if resp != nil && resp.Body != nil {
		if problem := resp.Body.Close(); problem != nil {
			Log(WARN, ctx, "core.Slurp", "url", url, "error", problem, "when", "close")
			if err == nil {
				err = problem
			}
		}
	}

	if err != nil {
		Log(WARN, ctx, "core.Slurp", "url", url, "error", err)
		return "", err
	}

	return string(body), nil
}

var SlurpCache = NewCache(SystemParameters.SlurpCacheSize, SystemParameters.SlurpCacheTTL)

// CachedSlurp gets the contents (string) at a URL using
// a little LRU cache.  The cache has a TTL of 5 seconds
// See the constants SlurpCacheSize and SlurpCacheTTLSecs.
func CachedSlurp(ctx *Context, url string) (string, error) {
	timer := NewTimer(ctx, "CachedSlurp")
	defer timer.Stop()

	Log(INFO, ctx, "core.CachedSlurp", "url", url)
	v, cached := SlurpCache.Get(url)
	if cached {
		Log(DEBUG, ctx, "core.CachedSlurp", "cacheHit", url)
		return v.(string), nil
	}

	Log(DEBUG, ctx, "core.CachedSlurp", "cacheMiss", url)
	s, err := Slurp(ctx, url)
	if err != nil {
		return "", err
	}
	SlurpCache.Add(url, s)

	return s, nil
}

// Suggested at https://github.com/robertkrimen/otto/issues/17.
func throwJavascript(value otto.Value, _ error) otto.Value {
	panic(value)
}

// CompileJavascript compiles a code with specified libraries
func CompileJavascript(ctx *Context, loc *Location, libraries []string, code string) (*otto.Script, error) {
	if loc != nil {
		libCode, err := loc.getLibraryCode(ctx, libraries)
		if nil != err {
			return nil, NewSyntaxError(err.Error())
		}
		code = libCode + "\n" + code
	}

	runtime := getVM()
	defer returnVM(runtime)

	if SystemParameters.ScopedJavascriptRuntimes {
		code = fmt.Sprintf("(function(){%s})()", code)
	}
	script, err := runtime.Compile("", code)
	if nil != err {
		Log(WARN, ctx, "core.compileJavascript", "code", code, "error", err)
		return nil, NewSyntaxError(err.Error())
	}

	return script, nil
}

func LocationFunctions(ctx *Context, loc *Location, runtime *otto.Otto, env map[string]interface{}) {
	if loc == nil {
		Log(WARN, ctx, "Javascript.LocationFunctions", "location", "none")
		return
	}
	Log(DEBUG, ctx, "Javascript.LocationFunctions", "location", loc.Name)

	env["AddFact"] = func(call otto.FunctionCall) otto.Value {
		// id, object
		Log(DEBUG, ctx, "Javascript.AddFact")
		id, err := call.Argument(0).ToString()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No id (first arg) given"))
		}

		x, err := call.Argument(1).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No object (second arg) given"))
		}

		m, ok := x.(map[string]interface{})
		if !ok {
			throwJavascript(call.Otto.Call("new Error", nil, "Bad object (second arg) given"))
		}

		i, err := loc.AddFact(ctx, id, Map(m))
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(i)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["AddRule"] = func(call otto.FunctionCall) otto.Value {
		// id, object
		Log(DEBUG, ctx, "Javascript.AddRule")
		id, err := call.Argument(0).ToString()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No id (first arg) given"))
		}

		x, err := call.Argument(1).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No object (second arg) given"))
		}

		m, ok := x.(map[string]interface{})
		if !ok {
			throwJavascript(call.Otto.Call("new Error", nil, "Bad object (second arg) given"))
		}

		i, err := loc.AddRule(ctx, id, Map(m))
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(i)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["ProcessEvent"] = func(call otto.FunctionCall) otto.Value {
		// id, object
		Log(DEBUG, ctx, "Javascript.ProcessEvent")
		x, err := call.Argument(0).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No object (first arg) given"))
		}

		m, ok := x.(map[string]interface{})
		if !ok {
			throwJavascript(call.Otto.Call("new Error", nil, "Bad object (first arg) given"))
		}

		ews, err := loc.ProcessEvent(ctx, Map(m))
		if err != (*Condition)(nil) {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(ews)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["Search"] = func(call otto.FunctionCall) otto.Value {
		// object
		Log(DEBUG, ctx, "Javascript.Search")
		x, err := call.Argument(0).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No object (first arg) given"))
		}

		m, ok := x.(map[string]interface{})
		if !ok {
			throwJavascript(call.Otto.Call("new Error", nil, "Bad object (first arg) given"))
		}

		srs, err := loc.SearchFacts(ctx, m, true)

		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(srs)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["Query"] = func(call otto.FunctionCall) otto.Value {
		// object
		Log(DEBUG, ctx, "Javascript.Query")
		m, err := call.Argument(0).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No object (first arg) given"))
		}

		js, err := json.Marshal(&m)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "Bad object (first arg) given"))
		}

		qr, err := loc.Query(ctx, string(js))
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(qr)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["RemFact"] = func(call otto.FunctionCall) otto.Value {
		// id
		Log(DEBUG, ctx, "Javascript.RemFact")
		id, err := call.Argument(0).ToString()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No id (first arg) given"))
		}

		i, err := loc.RemFact(ctx, id)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(i)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}

	env["RemRule"] = func(call otto.FunctionCall) otto.Value {
		// id
		Log(DEBUG, ctx, "Javascript.RemRule")
		id, err := call.Argument(0).ToString()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, "No id (first arg) given"))
		}

		i, err := loc.RemRule(ctx, id)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		result, err := runtime.ToValue(i)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		return result
	}
}

func jsFun_http(ctx *Context, runtime *otto.Otto, env map[string]interface{}) func(call otto.FunctionCall) otto.Value {
	return func(call otto.FunctionCall) otto.Value {

		// Args: method, url, body, contentType, opts

		timer := NewTimer(ctx, "jsFun_http")
		defer timer.Stop()

		method, err := call.Argument(0).ToString()
		// Don't know how to throw an exception.
		if err != nil {
			v, _ := runtime.ToValue("No method (first arg) given.")
			return v

		}

		urlStr, err := call.Argument(1).ToString()
		if err != nil {
			v, _ := runtime.ToValue("No url (second arg) given.")
			return v
		}

		requestBody, err := call.Argument(2).ToString()
		if err != nil {
			v, _ := runtime.ToValue("No body (third arg) given.")
			return v
		}

		if requestBody == "undefined" {
			requestBody = ""
		}

		contentType, err := call.Argument(3).ToString()
		if err != nil || contentType == "undefined" {
			contentType = "application/json"
		}

		// opts := call.Argument(4).Object()

		req := NewHTTPRequest(ctx, method, urlStr, requestBody)
		req.ContentType = contentType

		res, err := req.Do(ctx)
		if nil != err {
			Log(ERROR, ctx, "jsFun_http", "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, res.Error))
		}

		result, err := runtime.ToValue(res.Body)
		if err != nil {
			Log(ERROR, ctx, "jsFun_http", "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		Log(DEBUG, ctx, "jsFun_http", "got", result)

		return result
	}
}

func jsFun_httpx(ctx *Context, runtime *otto.Otto, env map[string]interface{}) func(call otto.FunctionCall) otto.Value {
	return func(call otto.FunctionCall) otto.Value {

		// Args: request

		timer := NewTimer(ctx, "jsFun_httpx")
		defer timer.Stop()

		spec, err := call.Argument(0).Export()
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		if spec == nil {
			throwJavascript(call.Otto.Call("new Error", nil, "no spec arg"))
		}

		// Shameful
		js, err := json.Marshal(&spec)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		Log(DEBUG, ctx, "jsFun_httpx", "spec", string(js))

		var request HTTPRequest
		request.Method = "GET"
		request.ContentType = "application/json"
		if err = json.Unmarshal(js, &request); err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		var result HTTPResult
		if err = request.DoOnce(ctx, &result); err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}

		v, err := runtime.ToValue(&result)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return v
	}

}

type CommandSpec struct {
	Path    string            `json:"path"`
	Args    []string          `json:"args"`
	Dir     string            `json:"dir"`
	CGroup  string            `json:"cgroup"`
	AddEnv  map[string]string `json:"addEnv"`
	Env     map[string]string `json:"env"`
	Stdin   string            `json:"stdin"`
	Stdout  string            `json:"stdout"`
	Stderr  string            `json:"stderr"`
	Error   string            `json:",omit"`
	Success bool              `json:",success"`
	*os.ProcessState
}

func (cs *CommandSpec) Set(x interface{}) error {
	js, err := json.Marshal(&x)
	if err != nil {
		return err
	}
	return json.Unmarshal(js, cs)
}

func (cs *CommandSpec) Exec(ctx *Context) error {
	args := make([]string, 0, len(cs.Args)+1)
	args = append(args, cs.Path)
	args = append(args, cs.Args...)

	var env []string
	if cs.Env != nil {
		for p, v := range cs.Env {
			env = append(env, p+"="+v)
		}
	} else if cs.AddEnv != nil {
		env = os.Environ()
		for p, v := range cs.AddEnv {
			env = append(env, p+"="+v)
		}
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	cmd := exec.Cmd{
		Path:   cs.Path,
		Args:   args,
		Dir:    cs.Dir,
		Env:    env,
		Stdin:  strings.NewReader(cs.Stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	err := cmd.Run()
	cs.Success = err == nil
	if err != nil {
		cs.Error = err.Error()
	}
	cs.Stdout = stdout.String()
	cs.Stderr = stderr.String()
	cs.ProcessState = cmd.ProcessState
	return err
}

// JavascriptTestValue is returned by the Javascript function "testWithSystemParam".
//
// Motivation: A simple test of retrying an action that failed
// initially.  Example: Set JavascriptTestValue to something that'll
// cause a problem in an action.  Submit an event to trigger the
// action, which fails.  Change JavascriptTestValue to something
// agreeable.  Re-attempt the event work and celebrate sweet victory.
var JavascriptTestValue interface{}

// RunJavascript executes Javascript code with the given bindings.  A
// new environment is created for each call.  That environment
// contains several bindings.  See
// http://github.com/Comcast/rulio/blob/master/doc/Manual.md#in-process-javascript-actions
// for details.
//
// Currently the Javascript implementation is
// https://github.com/robertkrimen/otto.  We might also eventually
// support https://code.google.com/p/v8/ .
func RunJavascript(ctx *Context, bs *Bindings, props map[string]interface{}, src interface{}) (interface{}, error) {
	timer := NewTimer(ctx, "RunJavascript")
	defer timer.Stop()
	Log(DEBUG, ctx, "core.RunJavascript", "code", src)

	// https://github.com/robertkrimen/otto#otto
	env := make(map[string]interface{})
	envBindings := make(map[string]interface{})

	// if we're using reusable scoped runtimes and we're given uncompiled code, it needs to be wrapped
	if srcString, ok := src.(string); ok && SystemParameters.ScopedJavascriptRuntimes {
		src = fmt.Sprintf("(function(){%s})()", srcString)
	}

	runtime := getVM()
	defer returnVM(runtime)

	if ctx != nil && ctx.App != nil {
		if err := ctx.App.UpdateJavascriptRuntime(ctx, runtime); err != nil {
			return nil, err
		}
	}

	if bs != nil {
		for k, v := range *bs {
			Log(DEBUG, ctx, "core.RunJavascript", "var", k, "val", Gorep(v), "type", fmt.Sprintf("%T", v))
			if err := runtime.Set(k, v); err != nil {
				Log(WARN, ctx, "core.RunJavascript", "var", k, "val", Gorep(v), "when", "Set",
					"error", err)
				return nil, err
			}
			envBindings[k] = v
		}
	}

	// if we're reusing runtimes, give a halfhearted attempt to clean up the stuff we can.
	// notably, this does not include variables or global state changes created by code blocks
	if SystemParameters.ScopedJavascriptRuntimes && bs != nil {
		defer func() {
			for k := range *bs {
				_ = runtime.Set(k, otto.UndefinedValue())
			}
		}()
	}

	env["bindings"] = envBindings

	env["sleep"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "sleep", "call", call)
		ns, _ := call.Argument(0).ToInteger()
		time.Sleep(time.Duration(ns) * time.Nanosecond)
		return call.Argument(0)
	}

	var ctl *Control
	if ctx != nil {
		loc := ctx.GetLoc()
		if loc != nil {
			ctl = loc.Control()
		}
	}
	if ctl == nil || !ctl.DisableExecFunction {
		env["exec"] = func(call otto.FunctionCall) otto.Value {
			// First arg is a object that specifies a CommandSpec.
			Log(DEBUG, ctx, "core.RunJavascript", "f", "exec", "call", call)
			x, err := call.Argument(0).Export()
			if err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "exec", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			cs := CommandSpec{}
			if err = cs.Set(x); err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "exec", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			cs.Exec(ctx)
			result, err := runtime.ToValue(&cs)
			if err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "exec", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			return result
		}
	}

	if ctl == nil || !ctl.DisableExecFunction {
		env["bash"] = func(call otto.FunctionCall) otto.Value {
			// First arg is a string that'll be given to "bash -c".
			// Second optional argument is command options.

			Log(DEBUG, ctx, "core.RunJavascript", "f", "bash", "call", call)
			s, err := call.Argument(0).ToString()
			if err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "bash", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			cs := CommandSpec{}
			opts, err := call.Argument(1).Export()
			if err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "exec", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			if opts != nil {
				if err = cs.Set(opts); err != nil {
					Log(WARN, ctx, "core.RunJavascript", "f", "exec", "call", call, "error", err)
					throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
				}
			}

			bash := "/bin/bash"
			cs.Path = bash
			cs.Args = []string{"-c", s}

			cs.Exec(ctx)
			result, err := runtime.ToValue(&cs)
			if err != nil {
				Log(WARN, ctx, "core.RunJavascript", "f", "bash", "call", call, "error", err)
				throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
			}
			return result
		}
	}

	// Slurp with a cache
	env["getc"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "getc", "call", call)
		s, _ := call.Argument(0).ToString()
		body, err := CachedSlurp(ctx, s)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "getc", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		result, err := runtime.ToValue(body)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "get", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return result
	}

	// Slurp without a cache
	env["get"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "get", "call", call)
		s, _ := call.Argument(0).ToString()
		body, err := Slurp(ctx, s)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "get", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		result, err := runtime.ToValue(body)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "get", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return result
	}

	env["encode"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "encode", "call", call)
		s, _ := call.Argument(0).ToString()
		body := url.QueryEscape(s)
		result, err := runtime.ToValue(body)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "encode", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return result
	}

	env["log"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "log")
		o := call.Argument(0)
		x, err := o.Export()
		if err == nil {
			m, ok := x.(map[string]interface{})
			if ok {
				args := make([]interface{}, 0, 2*len(m))
				args = append(args, "UserJavascript")
				for p, v := range m {
					args = append(args, p, v)
				}
				Log(INFO, ctx, args...)
			} else {
				err = fmt.Errorf("%v (%T) is not a map", x, x)
			}
		}

		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		result, _ := runtime.ToValue(true)
		return result
	}

	env["secsFromNow"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "encode", "call", call)
		s, err := call.Argument(0).ToString()
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "encode", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		then, err := time.Parse(time.RFC3339, s)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "encode", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		secs := time.Now().Sub(then).Seconds()
		result, err := runtime.ToValue(secs)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "encode", "call", call, "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return result
	}

	// For testing.  This function will send the given value on
	// the channel stored at ctx.Value("out") if any.
	env["out"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "out")
		var v interface{}
		if ctx != nil {
			v = ctx.Prop("out")
		}
		if nil != v {
			switch vv := v.(type) {
			case chan interface{}:
				x, err := call.Argument(0).Export()
				if err != nil {
					Log(WARN, ctx, "core.RunJavascript", "f", "out", "err", err)
				} else {
					Log(DEBUG, ctx, "core.RunJavascript", "f", "out", "sending", x)
					vv <- x
				}
			default:
				Log(WARN, ctx, "core.RunJavascript", "f", "out", "warning", "not a channel")
			}
		}
		return call.Argument(0)
	}

	// HTTP anything
	// method, url, body
	env["http"] = jsFun_http(ctx, runtime, env)

	// HTTP anything, but from a HTTPRequest given as first
	// argument.  Returns full HTTPResponse.
	env["httpx"] = jsFun_httpx(ctx, runtime, env)

	//
	env["getJavascriptTestValue"] = func(call otto.FunctionCall) otto.Value {
		x, err := runtime.ToValue(JavascriptTestValue)
		if err != nil {
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return x
	}

	env["match"] = func(call otto.FunctionCall) otto.Value {
		Log(DEBUG, ctx, "core.RunJavascript", "f", "match", "call", call)
		pat, err := call.Argument(0).Export()
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "0.Export", "warning", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		Log(DEBUG, ctx, "core.RunJavascript", "f", "match", "pat", pat, "type", fmt.Sprintf("%T", pat))
		fact, err := call.Argument(1).Export()
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "1.Export", "warning", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		Log(DEBUG, ctx, "core.RunJavascript", "f", "match", "facti", logFacti(fact), "type", fmt.Sprintf("%T", fact))
		bss, err := Matches(ctx, pat, fact)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "Matches", "warning", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		Log(DEBUG, ctx, "core.RunJavascript", "f", "match", "bss", bss)
		// matched := 0 < len(bss)
		// Log(DEBUG, ctx, "core.RunJavascript", "f", "match", "bss", bss, "matched", matched)
		result, err := runtime.ToValue(bss)
		if err != nil {
			Log(WARN, ctx, "core.RunJavascript", "f", "ToValue", "error", err)
			throwJavascript(call.Otto.Call("new Error", nil, err.Error()))
		}
		return result
	}

	if ctx != nil {
		LocationFunctions(ctx, ctx.GetLoc(), runtime, env)
	}

	if ctx != nil {
		env["Context"] = ctx
		env["TxId"] = ctx.Id()
	}

	if ctx != nil && ctx.GetLoc() != nil {
		env["Location"] = ctx.GetLoc().Name
	}

	// We want to support multiple versions of Javascript
	// environments, which just consist of the utility functions
	// above.  We'll put each version in, for example,
	// 'Envs["1.0"]'.
	envs := make(map[string]interface{})
	thisVersion := "1.0"

	// Let's make an convenient array of the versions we're supporting.
	versions := []string{thisVersion}
	// Put it in the base env.
	env["Versions"] = versions

	// Now let's define our versioned env (in addition to the
	// default, top-level env).
	thisEnv := make(map[string]interface{})
	thisEnv["Version"] = thisVersion
	for k, v := range env {
		thisEnv[k] = v
	}
	envs[thisVersion] = thisEnv

	// I guess we'll keep these props out of the versioned
	// environment.
	for k, v := range props {
		env[k] = v
	}

	if err := runtime.Set("Env", env); err != nil {
		Log(ERROR, ctx, "core.RunJavascript", "when", "Set(Env)", "error", err.Error())
		return nil, err
	}

	// Install the different env versions that we support.
	if err := runtime.Set("Envs", envs); err != nil {
		Log(ERROR, ctx, "core.RunJavascript", "when", "Set(Envs)", "error", err.Error())
		return nil, err
	}

	// Optionally time out the execution.
	//
	// Default is given by System.DefaultJavascriptTimeout.  Set
	// ctx.GetLoc().Control.JavascriptTimeout to override.
	// Negative means no timeout.  Costs an additional goroutine
	// and a defer.  Also is a little scary.  See
	// https://github.com/robertkrimen/otto#halting-problem .
	var timeout time.Duration
	if SystemParameters.JavascriptTimeouts && ctx != nil && ctx.GetLoc() != nil {
		c := ctx.GetLoc().Control()
		if c != nil {
			timeout = time.Duration(c.JavascriptTimeout)
			Log(DEBUG, ctx, "core.RunJavascript", "JavascriptTimeout", timeout)
		}
	}
	if timeout == 0 {
		timeout = SystemParameters.DefaultJavascriptTimeout
	}

	if SystemParameters.JavascriptTimeouts && 0 <= int64(timeout) {
		start := time.Now()
		Log(DEBUG, ctx, "core.RunJavascript", "timeout", timeout, "start", start)
		defer func() {
			duration := time.Since(start)
			if caught := recover(); caught != nil {
				if caught == Halt {
					Log(WARN, ctx, "core.RunJavascript", "timedout", timeout,
						"after", duration, "time", time.Now())
					return
				}
				panic(caught) // Something else happened, so repanic!
			}
		}()

		runtime.Interrupt = make(chan func(), 1) // No blocking
		go func() {
			time.Sleep(timeout)
			runtime.Interrupt <- func() {
				panic(Halt)
			}
		}()
	}

	v, err := runtime.Run(src)
	if err != nil {
		Log(ERROR, ctx, "core.RunJavascript", "error", err.Error(), "when", "runtime.Run")
		return nil, err
	}
	x, err := v.Export()
	if err != nil {
		Log(ERROR, ctx, "core.RunJavascript", "error", err.Error(), "when", "v.Export")
		return nil, err
	}
	return x, err
}
