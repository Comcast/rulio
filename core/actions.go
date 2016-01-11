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
	"regexp"
	"strings"
)

// Action is something that a rule performs.
type Action struct {
	// Code is optional Javascript.
	//
	// Can either be an array of strings or a string.
	Code interface{} `json:"code,omitempty"`

	// Endpoint is the optional target action executor.
	Endpoint string `json:"endpoint,omitempty"`

	// Subvars controls whether bindings are injected directly
	// into the Javascript environment.
	//
	// For example, if the rule evaluation results in a binding
	// for "foo" and if Subvars is true, then the Javascript
	// variable 'foo' will be bound.
	Subvars bool `json:"subvars,omitempty"`

	// Opts is a map of generic options.
	//
	// For now, only "libraries" is used.  "libraries", if given,
	// should be an array of URLs that return Javascript.
	Opts map[string]interface{} `json:"opts,omitempty"`
}

type CleanAction Action

func (a *CleanAction) UnmarshalJSON(bs []byte) error {
	x := &Action{}
	x.Subvars = true
	if err := json.Unmarshal(bs, x); err != nil {
		return err
	}
	if x.Endpoint == "" {
		x.Endpoint = "javascript"
	}
	var err error
	if x.Code, err = convertCode(x.Code); err != nil {
		return err
	}

	a.Code = x.Code
	a.Endpoint = x.Endpoint
	a.Subvars = x.Subvars
	a.Opts = x.Opts

	return nil
}

func SubstituteBindings(ctx *Context, code string, bs Bindings) (interface{}, error) {
	var o interface{}

	if err := json.Unmarshal([]byte(code), &o); nil != err {
		return "", err
	}

	x, err := substituteInterface(ctx, o, bs)
	if err != nil {
		return nil, err
	}

	return CoerceFakeFloats(x), nil
}

// substituteInterface attempts to traverse the given thing to replace
// variable references with their corresponding bindings.
//
// The main work is performed by the mysterious 'substituteString'.
func substituteInterface(ctx *Context, src interface{}, bs Bindings) (o interface{}, err error) {
	switch src.(type) {
	case string:
		return substituteString(ctx, src.(string), bs)
	case []interface{}:
		vs := make([]interface{}, len(src.([]interface{})))
		for i, v := range src.([]interface{}) {
			switch v.(type) {
			case string:
				if vs[i], err = substituteString(ctx, v.(string), bs); nil != err {
					return nil, err
				}
			default:
				if vs[i], err = substituteInterface(ctx, v, bs); nil != err {
					return nil, err
				}
			}
		}
		return vs, nil
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, v := range src.(map[string]interface{}) {
			// A substitution of a map property could end
			// up being a non-string.  If so, we protest.
			ki, err := substituteString(ctx, k, bs)
			if err != nil {
				return nil, err
			}
			ks, ok := ki.(string)
			if !ok {
				err = fmt.Errorf("substitution of %s was %#v, which isn't a string", k, ki)
				return nil, err
			}
			k = ks
			if m[k], err = substituteInterface(ctx, v, bs); nil != err {
				return nil, err
			}
		}
		return m, nil
	default:
		return src, nil
	}
}

var nakedVariablePattern = regexp.MustCompile("^\\?[_a-zA-Z][_0-9a-zA-Z]*$")

func IsNakedVariable(s string) bool {
	return nakedVariablePattern.MatchString(s)
}

// substituteString attempts to replace variable references in the
// given string with their corresponding bindings.
//
// If the given string is an exact match with a bound variable, then
// the binding is returned.  Otherwise a more mysterious process
// occurs that replaces variable references in a poorly defined way.
func substituteString(ctx *Context, src string, bs Bindings) (interface{}, error) {
	for p, v := range bs {
		if src == p {
			// We have an exact match.  That's important
			// for cases like issue 303, which, among
			// other things, ran into a problem of
			// substituting an number INSIDE a string
			// leaves it as a true string (in Go).
			return v, nil
		}
	}
	// Check for an unbound but alone variable.
	if IsNakedVariable(src) {
		if ctx != nil && ctx.GetLoc() != nil {
			ctl := ctx.GetLoc().Control()
			if ctl.UseDefaultVariableValue {
				return ctl.DefaultVariableValue, nil
			}
		}
		return src, fmt.Errorf("naked variable '%s' unbound", src)
	}
	for p, v := range bs {
		var js string
		switch v.(type) {
		case string:
			js = v.(string)
		default:
			if bts, err := json.Marshal(&v); nil != err {
				js = fmt.Sprintf(`%v`, v)
			} else {
				js = string(bts)
			}
		}

		if re, err := regexp.Compile("\\" + p + "\\b"); nil != err {
			return "", err
		} else {
			src = re.ReplaceAllString(src, js)
		}
	}

	return src, nil
}

func dropQuestionMarks(bindings map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for p, v := range bindings {
		if strings.HasPrefix(p, "?") {
			p = p[1:]
		}
		m[p] = v
	}
	return m
}

func (loc *Location) ExecAction(ctx *Context, bs Bindings, a Action) (interface{}, error) {

	Log(INFO, ctx, "core.ExecAction", "action", a)

	f, err := loc.getActionFunc(ctx, bs, a)

	if nil != err {
		return nil, err
	}

	timer := NewTimer(ctx, "ExecAction")
	x, err := f()
	if err != nil {
		Log(ERROR, ctx, "core.ExecAction", "action", a, "error", err)
	} else {
		Log(DEBUG, ctx, "core.ExecAction", "action", a, "got", x)
	}
	timer.Stop()
	return x, err
}

func (loc *Location) getActionFunc(ctx *Context, bs Bindings, a Action) (func() (interface{}, error), error) {
	Log(DEBUG, ctx, "core.getActionFunc", "action", a)

	if a.Endpoint == "javascript" {
		var libraries []string

		if o, given := a.Opts["libraries"]; given {
			switch vv := o.(type) {
			case []string:
				libraries = vv
			case []interface{}:
				acc := make([]string, len(vv))
				for i, lib := range vv {
					s, ok := lib.(string)
					if !ok {
						err := fmt.Errorf("bad library type: %T (%#v)", lib, lib)
						Log(ERROR|USR, ctx, "core.getActionFunc", "error", err)
						return nil, err
					}
					acc[i] = s
				}
				libraries = acc
			default:
				err := fmt.Errorf("bad 'libraries' type: %T (%#v)", o, o)
				Log(ERROR|USR, ctx, "core.getActionFunc", "error", err)
				return nil, err
			}
		}

		script, err := CompileJavascript(ctx, loc, libraries, a.Code.(string))
		if nil != err {
			return nil, err
		}

		f := func() (interface{}, error) {
			Log(DEBUG, ctx, "core.getActionFunc.javascript",
				"action", a, "stage", "start")

			var props map[string]interface{}
			c := loc.Control()
			if c != nil && c.CodeProps != nil {
				Log(DEBUG, ctx, "core.getActionFunc.javascript",
					"action", a, "CodeProps", c.CodeProps)
				props = c.CodeProps
			} else {
				Log(DEBUG, ctx, "core.getActionFunc.javascript",
					"action", a, "CodeProps", nil)
			}

			v, err := RunJavascript(ctx, bs.StripQuestionMarks(ctx), props, script)
			if err != nil {
				// Don't know if this error is a user error.
				// Probably a user error.  We'll log both for now.
				Log(ERROR|USR, ctx, "core.getActionFunc.javascript", "action", a, "error", err)
				return nil, err
			}
			Log(DEBUG, ctx, "core.getActionFunc.javascript",
				"action", a, "stage", "done")
			return v, err
		}

		return f, nil
	}

	endpoint, err := loc.ResolveService(ctx, a.Endpoint)
	if err != nil {
		// return nil, &rest.SyntaxError{err.Error()}
		// Not a syntax error?
		return nil, err
	}

	if strings.HasPrefix(endpoint, "http:") || strings.HasPrefix(endpoint, "https:") {
		// Only supporting POST for now.
		m := make(map[string]interface{})

		m["bindings"] = bs
		m["opts"] = a.Opts

		if a.Subvars {
			if m["code"], err = SubstituteBindings(ctx, a.Code.(string), bs); nil != err {
				return nil, NewSyntaxError(err.Error())
			}
		} else {
			m["code"] = a.Code
		}

		bts, err := json.Marshal(m)
		if nil != err {
			return nil, NewSyntaxError(err.Error())
		}

		if err != nil {
			Log(WARN, ctx, "core.getActionFunc", "error", err, "when", "bodyFromMap", "map", m)
			return nil, NewSyntaxError(err.Error())
		}

		f := func() (interface{}, error) {
			js := string(bts)
			Log(DEBUG, ctx, "core.getActionFunc.post", "body", js)
			if o, err := Post(ctx, endpoint, "application/json", js); nil != err {
				Log(ERROR, ctx, "core.getActionFunc.post", "error", err)
				return nil, err
			} else {
				// ToDo: Attempt parse?
				return o, nil
			}
		}

		return f, nil
	}

	return nil, fmt.Errorf("Unsupported action endpoint '%s' (given '%s')", endpoint, a.Endpoint)
}

func convertCode(x interface{}) (string, error) {
	switch vv := x.(type) {
	case string:
		return vv, nil

	case []string:
		var acc string
		for _, s := range vv {
			acc += s + "\n"
		}
		return acc, nil

	case []interface{}:
		var acc string
		for _, y := range vv {
			s, ok := y.(string)
			if !ok {
				return "", fmt.Errorf("bad code %#v (%T)", y, y)
			}
			acc += s + "\n"
		}
		return acc, nil

	case map[string]interface{}:
		// Hack for language="http"
		c := x.(map[string]interface{})
		js, err := json.Marshal(&c)
		if err != nil {
			return "", NewSyntaxError(err.Error())
		}
		return string(js), nil
	}

	return "", fmt.Errorf("bad code %#v (%T)", x, x)
}

func ActionFromMap(ctx *Context, m map[string]interface{}) (*Action, error) {
	Log(DEBUG, ctx, "core.ActionFromMap", "map", m)

	bs, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	a := &Action{}
	if err = json.Unmarshal(bs, a); err != nil {
		return nil, err
	}

	return a, nil
}
