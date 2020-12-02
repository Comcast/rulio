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
	"fmt"
	"strings"
	"time"
)

// Replace all variables in the given pattern with their bindings
// (when possible).
func (bs *Bindings) Bind(ctx *Context, pat interface{}) interface{} {
	switch v := pat.(type) {
	case string:
		if isVariable(v) {
			binding, found := (*bs)[v]
			if found {
				return interface{}(binding)
			} else {
				return pat
			}
		} else {
			return v
		}
	case map[string]interface{}:
		bound := make(map[string]interface{})
		for k, x := range v {
			bound[k] = bs.Bind(ctx, x)
		}
		return bound

	case []interface{}:
		bound := make([]interface{}, 0, len(v))
		for _, x := range v {
			bound = append(bound, bs.Bind(ctx, x))
		}
		return bound

	default:
		return pat
	}
}

type QueryResult struct {
	Bss     []Bindings
	Checked int
	Elapsed int64
}

// Make a QueryResult that contains one empty Bindings.
func InitialQueryResult(ctx *Context) QueryResult {
	qr := QueryResult{make([]Bindings, 0, 1), 0, 0}
	qr.Bss = append(qr.Bss, make(map[string]interface{}))
	return qr

}

type QueryContext struct {
	// Or maybe map[string]interface{} ...
	Locations []string
}

type Query interface {
	Exec(*Context, *Location, QueryContext, QueryResult) (*QueryResult, error)
}

func ExecQuery(ctx *Context, q Query, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	timer := NewTimer(ctx, "ExecQuery")
	r, err := q.Exec(ctx, loc, qc, qr)
	if r != nil {
		r.Elapsed += timer.Stop()
	}
	return r, err
}

type CodeQuery struct {
	Code         string   `json:"code"`
	Language     string   `json:"language,omitempty"`
	Libraries    []string `json:"libraries,omitempty"`
	compiledCode interface{}
}

// Strips '?' from variables.  Warn if no '?'.
func (bs *Bindings) StripQuestionMarks(ctx *Context) *Bindings {
	acc := make(Bindings)
	for k, v := range *bs {
		if len(k) == 0 {
			Log(DEBUG, ctx, "Bindings.StripQuestionMarks", "warning", "nil variable", "bs", bs)
			continue
		}
		c := k[0]
		if c != '?' {
			Log(DEBUG, ctx, "Bindings.StripQuestionMarks", "warning", "unmarked variable", "k", k)
			acc[k] = v
		} else {
			acc[k[1:]] = v
		}
	}
	return &acc
}

func isURL(library string) bool {
	// ToDo: Do better.
	return strings.HasPrefix(library, "http")
}

// fetchLibrary attempts to obtain code for the given library.
//
// The given library can be a URL, a name that resolves to a URL via
// location.GetConfig().Libraries, or even explicit Javascript code.
func (loc *Location) fetchLibrary(ctx *Context, library string) (string, error) {
	var uri string
	var comment string
	if isURL(library) {
		uri = library
		comment += fmt.Sprintf("// Library is URL '%s'\n", library)
		Log(INFO, ctx, "core.fetchLibrary", "library", library, "isURL", true)
	} else {
		libs := loc.Control().Libraries
		if libs == nil {
			err := fmt.Errorf("No Libraries for library '%s'", library)
			Log(ERROR|USR, ctx, "core.fetchLibrary", "library", library, "error", err)
			return "", err
		}
		uri, _ = libs[library]
	}

	var code string
	if isURL(uri) {
		Log(INFO, ctx, "core.fetchLibrary", "library", library, "uri", uri)
		var err error
		code, err = CachedSlurp(ctx, uri)
		if err != nil {
			Log(ERROR|APP, ctx, "core.fetchLibrary", "uri", uri,
				"error", err, "when", "slurp", "library", library)
			return "", err
		}
	} else {
		// For now, not an error.
		// Better be code.
		Log(DEBUG, ctx, "core.fetchLibrary", "library", library, "code", true)
		comment += fmt.Sprintf("// Library is explicit code\n")
		code = uri
	}

	return comment + "\n" + code, nil
}

// getLibraryCode gathers up all library code.
//
// The code is concatenated (in order) and returned.  Uses
// 'fetchLibrary()' for resolution.
func (loc *Location) getLibraryCode(ctx *Context, libraries []string) (string, error) {
	var code string

	for i, library := range libraries {
		moreCode, err := loc.fetchLibrary(ctx, library)
		if err != nil {
			return "", err
		}
		code += fmt.Sprintf("// Library %s (%d)\n", library, i)
		code += moreCode
		code += fmt.Sprintf("// Library %s (%d) end\n\n", library, i)
	}
	Log(INFO, ctx, "core.getLibraryCode", "foundChars", len(code))
	return code, nil
}

func (c *CodeQuery) GetCompiledCode(ctx *Context, loc *Location) (interface{}, error) {
	var err error
	if nil == c.compiledCode {
		c.compiledCode, err = CompileJavascript(ctx, loc, c.Libraries, c.Code)
	}

	return c.compiledCode, err
}

func (c CodeQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	Log(INFO, ctx, "CodeQuery.Exec")
	timer := NewTimer(ctx, "CodeQuery.Exec")
	defer timer.Stop()

	acc := QueryResult{make([]Bindings, 0, 0), qr.Checked, qr.Elapsed}

	script, err := c.GetCompiledCode(ctx, loc)
	if err != nil {
		return nil, err
	}
	// ToDo: Somehow reuse Javascript runtimes to avoid re-loading code.

	for _, bs := range qr.Bss {
		Log(DEBUG, ctx, "CodeQuery.Exec", "script", script, "bs", bs)

		var props map[string]interface{}
		if loc != nil {
			c := loc.Control()
			if c != nil && c.CodeProps != nil {
				props = c.CodeProps
			}
		}

		maybeCopyEvent(bs)

		x, err := RunJavascript(ctx, bs.StripQuestionMarks(ctx), props, script)
		if err != nil {
			Log(WARN, ctx, "CodeQuery.Exec", "error", err)
			return nil, err
		}

		Log(DEBUG, ctx, "CodeQuery.Exec", "got", Gorep(x), "type", fmt.Sprintf("%T", x))
		switch vv := x.(type) {
		case bool:
			if vv == true {
				acc.Bss = append(acc.Bss, bs)
			}
		case map[string]interface{}:
			// Additional bindings!
			more := make(map[string]interface{})
			for p, v := range bs {
				more[p] = v
			}
			for p, v := range vv {
				more["?"+p] = v
				Log(DEBUG, ctx, "CodeQuery.Exec", "binding", p, "value", v)
			}
			acc.Bss = append(acc.Bss, more)
		default:
			if x != nil {
				acc.Bss = append(acc.Bss, bs)
			}
		}
	}
	return &acc, nil
}

func getLocationsFromMap(ctx *Context, m map[string]interface{}) ([]string, error) {
	var locations []string
	location, given := m["location"]
	if given {
		s, ok := location.(string)
		if !ok {
			return nil, fmt.Errorf("%#v is not a string", location)
		}
		locations = append(locations, s)
	}

	ls, given := m["locations"]
	if given {
		switch locs := ls.(type) {
		case []interface{}:
			for _, l := range locs {
				loc, ok := l.(string)
				if !ok {
					return nil, fmt.Errorf("location %#v should be a string", l)
				}
				locations = append(locations, loc)
			}
		case []string:
			for _, loc := range locs {
				locations = append(locations, loc)
			}
		default:
			return nil, fmt.Errorf("locations %#v should be an array of strings", ls)
		}
	}
	return locations, nil
}

func CodeQueryFromMap(ctx *Context, m map[string]interface{}) (Query, bool, error) {
	var q CodeQuery
	code, ok := m["code"]
	if !ok {
		return nil, false, nil
	}
	switch vv := code.(type) {
	case string:
		q.Code = vv
	case []interface{}:
		for _, x := range vv {
			s, ok := x.(string)
			if !ok {
				return nil, false, fmt.Errorf("%#v is not a string", code)
			}
			q.Code += s + "\n"
		}
	case []string:
		for _, s := range vv {
			q.Code += s + "\n"
		}
	default:
		return nil, false, fmt.Errorf("%#v is not a string", code)
	}

	q.Libraries = make([]string, 0, 0)
	libraries, ok := m["libraries"]
	if ok {
		switch vv := libraries.(type) {
		case []interface{}:
			for _, library := range vv {
				switch s := library.(type) {
				case string:
					q.Libraries = append(q.Libraries, s)
				default:
					err := fmt.Errorf("library should be a string, not %T (%#v)",
						library, library)
					Log(DEBUG, ctx, "core.CodeQueryFromMap", "error", err)
					return nil, true, err
				}
			}
		default:
			err := fmt.Errorf("libraries should be an array of strings, not %T (%#v)",
				libraries, libraries)
			Log(ERROR, ctx, "core.CodeQueryFromMap", "error", err)
			return nil, true, err
		}
	}
	Log(DEBUG, ctx, "core.CodeQueryFromMap", "libraries", q.Libraries)

	// Only for validation:
	var location *Location
	// Won't have a ctx due to UnmarshalJSON path.
	if ctx != nil {
		location = ctx.GetLoc()
	}
	_, err := q.GetCompiledCode(ctx, location)
	if nil != err {
		return nil, true, err
	}

	Log(DEBUG, ctx, "core.CodeQueryFromMap", "query", q)
	return q, true, nil
}

type EmptyQuery struct {
}

func (p EmptyQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	Log(DEBUG, ctx, "EmptyQuery.Exec", "qr", qr, "Bss", qr.Bss)
	acc := QueryResult{qr.Bss, qr.Checked, qr.Elapsed}
	return &acc, nil
}

type PatternQuery struct {
	Pattern   map[string]interface{} `json:"pattern"`
	Locations []string               `json:"locations,omitempty"`
}

type FactService interface {
	Search(ctx *Context, pattern Map) (*SearchResults, error)
}

// Want anonymous interface implementations.

type FactServiceFromURI struct {
	URI     string
	Timeout time.Duration
}

func (s *FactServiceFromURI) Search(ctx *Context, pattern Map) (*SearchResults, error) {
	op := "core.FactServiceFromURI.Search"
	Log(DEBUG, ctx, op, "pattern", pattern)

	js, err := json.Marshal(&pattern)
	if err != nil {
		return nil, err
	}

	then := Now()
	body, err := Post(ctx, s.URI, "application/json", string(js))
	if err != nil {
		Log(ERROR, ctx, op, "error", err, "when", "Post")
		return nil, err
	}
	Log(DEBUG, ctx, op, "response", body)

	srs := new(SearchResults)
	err = json.Unmarshal([]byte(body), srs)
	// Overwrite .Elapsed?  FS should use another key.
	srs.Elapsed = Now() - then

	if err != nil {
		Log(ERROR, ctx, op, "error", err, "when", "unmarshall", "json", string(body))
		return nil, err
	}

	return srs, nil
}

// External fact service access
func (loc *Location) SearchRemoteFacts(ctx *Context, target string, pattern Map) (*SearchResults, error) {

	op := "core.SearchFactsAtRemoteLocation"
	Log(INFO, ctx, op, "target", target, "pattern", pattern)

	// First check to see if we have an internal fact service.
	c := loc.Control()
	if c != nil {
		ifss := c.InternalFactServices
		if ifss != nil {
			ifs, have := ifss[target]
			Log(DEBUG, ctx, op, "invoking", have)
			if have {
				return ifs.Search(ctx, pattern)
			}
		}
	}

	// External fact service.
	target, err := loc.ResolveService(ctx, target)
	if err != nil {
		return nil, err
	}
	Log(DEBUG, ctx, op, "resolved", target)

	timeout := 10 * time.Second
	ctl := loc.Control()
	if ctl != nil {
		timeout = time.Duration(ctl.ExternalFactServiceTimeout)
	}
	Log(INFO, ctx, op, "timeout", timeout)

	fs := FactServiceFromURI{target, timeout}

	return fs.Search(ctx, pattern)
}

func (loc *Location) SearchLocations(ctx *Context, locations []string, pattern map[string]interface{}) (*SearchResults, error) {
	Log(INFO, ctx, "Location.SearchLocation", "locations", locations, "pattern", pattern)

	if len(locations) == 0 {
		Log(WARN, ctx, "Location.SearchLocation", "locations", "none provided")
		locations = append(locations, loc.Name)
	}

	srs := new(SearchResults)
	srs.Found = make([]SearchResult, 0, 0)
	for _, location := range locations {
		var more *SearchResults
		var err error
		if location == loc.Name {
			more, err = loc.SearchFacts(ctx, pattern, true)
		} else {
			more, err = loc.SearchRemoteFacts(ctx, location, pattern)
		}
		if err != nil {
			Log(ERROR, ctx, "System.SearchLocation", "error", err)
			return nil, err
		}
		for _, sr := range more.Found {
			Log(DEBUG, ctx, "System.SearchLocations", "sr", sr)
			srs.Found = append(srs.Found, sr)
		}
		srs.Checked += more.Checked
	}
	Log(DEBUG, ctx, "System.SearchLocations", "locations", locations, "pattern", pattern, "results", *srs)
	return srs, nil
}

// Copy the given bindings into a new set of bindings.
func ExtendBindings(ctx *Context, x *Bindings, y *Bindings) *Bindings {
	acc := make(Bindings)
	for k, v := range *x {
		acc[k] = v
	}
	for k, v := range *y {
		prev, have := acc[k]
		if have {
			Log(DEBUG, ctx, "core.ExtendBindings", "x", *x, "y", *y, "warning",
				fmt.Sprintf("replacing %v=%v with %v in %v", k, prev, v, acc))
		}
		acc[k] = v
	}
	return &acc
}

func (p PatternQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	Log(DEBUG, ctx, "PatternQuery.Exec", "locations", p.Locations, "pattern", p.Pattern, "qr", qr)
	acc := QueryResult{make([]Bindings, 0, 0), qr.Checked, qr.Elapsed}
	// ToDo: Add to elapsed in QueryResult
	for _, bs := range qr.Bss {
		bound := bs.Bind(ctx, interface{}(p.Pattern))

		// qc.locations are from the triggering event
		// locations for PatternQuery are used to get multiple bindgings for each location
		locations := p.Locations
		if len(locations) == 0 {
			locations = qc.Locations
		}

		m, ok := bound.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%#v isn't a map[string]interface{}", bound)
		}
		srs, err := loc.SearchLocations(ctx, locations, m)
		if err != nil {
			return nil, err
		}

		// create bindings for each location
		for _, sr := range srs.Found {
			for _, more := range sr.Bindingss {
				extended := ExtendBindings(ctx, &bs, &more)
				acc.Bss = append(acc.Bss, *extended)
			}
		}
	}
	Log(DEBUG, ctx, "PatternQuery.Exec", "results", acc)
	return &acc, nil
}

func PatternQueryFromMap(ctx *Context, m map[string]interface{}) (Query, bool, error) {
	var q PatternQuery
	pattern, ok := m["pattern"]
	if !ok {
		Log(DEBUG, ctx, "core.PatternQueryFromMap", "map", m, "msg", "no pattern")
		return nil, false, NewSyntaxError("No pattern in map")
	}
	arg, ok := pattern.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%#v isn't a map[string]interface{}", pattern)
	}
	q.Pattern = arg
	locs, err := getLocationsFromMap(ctx, m)
	if err != nil {
		return nil, false, err
	}
	q.Locations = locs

	return q, true, nil
}

type AndQuery struct {
	Conjuncts []Query
}

func (q AndQuery) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{"and":`)
	js, err := json.Marshal(&q.Conjuncts)
	if err != nil {
		return nil, err
	}
	buf.Write(js)
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

func (a AndQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	switch len(a.Conjuncts) {
	case 0:
		return &qr, nil
	default:
		for _, q := range a.Conjuncts {
			acc, err := q.Exec(ctx, loc, qc, qr)
			if nil != err {
				return nil, err
			}
			qr = *acc
		}
		return &qr, nil
	}
}

func AndQueryFromMap(ctx *Context, m map[string]interface{}) (Query, bool, error) {
	var q AndQuery
	x, givenAnd := m["and"]
	if !givenAnd {
		return nil, false, nil
	}
	xs, ok := x.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%#v isn't an []interface{}", x)
	}
	qs := make([]Query, 0, len(xs))
	for _, x := range xs {
		m, ok := x.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("%#v isn't a map[string]interface{}", x)
		}
		got, err := ParseQuery(ctx, m)
		if err != nil {
			Log(WARN, ctx, "core.AndQueryFromMap", "error", err, "when", "bad subquery")
			return nil, true, err
		}
		qs = append(qs, got)
	}
	q.Conjuncts = qs

	// locations, givenLocations := m["locations"]
	// if givenLocations {
	// 	for _, i := range locations.([]interface{}) {
	// 		q.locations = append(q.locations, i.(string))
	// 	}
	// } else {
	//      return nil, true, errors.New(fmt.Sprintf("Currently must provide 'locations' in %v.", m))
	// }

	Log(DEBUG, ctx, "core.AndQuery", "query", q)
	return q, true, nil
}

type OrQuery struct {
	Disjuncts    []Query
	ShortCircuit bool
}

func (q OrQuery) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{"or":`)
	js, err := json.Marshal(&q.Disjuncts)
	if err != nil {
		return nil, err
	}
	buf.Write(js)
	if q.ShortCircuit {
		buf.WriteString(`,"shortCircuit": true`)
	}
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

func (o OrQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {
	acc := QueryResult{make([]Bindings, 0, 0), qr.Checked, qr.Elapsed}
	for _, bs := range qr.Bss {
		qr := QueryResult{[]Bindings{bs}, 0, 0}
		for _, q := range o.Disjuncts {
			more, err := q.Exec(ctx, loc, qc, qr)
			if nil != err {
				return nil, err
			}
			acc.Bss = append(acc.Bss, more.Bss...)
			if o.ShortCircuit && len(more.Bss) > 0 {
				break
			}
		}
	}
	return &acc, nil
}

func OrQueryFromMap(ctx *Context, m map[string]interface{}) (Query, bool, error) {
	var q OrQuery
	x, givenOr := m["or"]
	if !givenOr {
		return nil, false, nil
	}
	xs, ok := x.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("%#v isn't a []interface{}", x)
	}
	qs := make([]Query, 0, len(xs))
	for _, x := range xs {
		m, ok := x.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("%#v isn't a map[string]interface{}", x)
		}
		got, err := ParseQuery(ctx, m)
		if err != nil {
			Log(WARN, ctx, "core.OrQueryFromMap", "error", err, "when", "Bad subquery")
			return nil, true, err
		}
		qs = append(qs, got)
	}
	q.Disjuncts = qs

	for _, candidate := range []string{"shortCircuit", "ShortCircuit", "short_circuit", "shortcircuit"} {
		sc, ok := m[candidate]
		if ok {
			scBool, ok := sc.(bool)
			if ok {
				q.ShortCircuit = scBool
				break
			}
			return nil, false, fmt.Errorf("field %s: %v isn't a bool", candidate, sc)
		}
	}

	return q, true, nil
}

type NotQuery struct {
	Negated Query
}

func (q NotQuery) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString(`{"not":`)
	js, err := json.Marshal(&q.Negated)
	if err != nil {
		return nil, err
	}
	buf.Write(js)
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

func (o NotQuery) Exec(ctx *Context, loc *Location, qc QueryContext, qr QueryResult) (*QueryResult, error) {

	// A little trick.  Probably wrong: For each current Bindings,
	// try the subquery.  If that subquery fails, then keep the
	// bindings.  Otherwise discard them.

	acc := make([]Bindings, 0, 0)
	for _, candidate := range qr.Bss {
		trial := QueryResult{[]Bindings{candidate}, 0, 0}
		more, err := o.Negated.Exec(ctx, loc, qc, trial)
		// ToDo: Add up Checked, Elapsed.
		if err != nil {
			return nil, err
		}
		if 0 == len(more.Bss) {
			acc = append(acc, candidate)
		}
	}

	return &QueryResult{acc, qr.Checked, qr.Elapsed}, nil
}

func NotQueryFromMap(ctx *Context, m map[string]interface{}) (Query, bool, error) {
	x, givenNot := m["not"]
	if !givenNot {
		return nil, false, nil
	}
	arg, ok := x.(map[string]interface{})
	if !ok {
		return nil, true, fmt.Errorf("not takes a single argument (a map), not a %T", x)
	}
	subquery, err := ParseQuery(ctx, arg)
	if err != nil {
		return nil, true, err
	}
	q := NotQuery{subquery}

	return q, true, nil
}

func ParseQuery(ctx *Context, m map[string]interface{}) (Query, error) {
	Log(DEBUG, ctx, "core.ParseQuery", "map", fmt.Sprintf("%#v", m))
	if len(m) == 0 {
		return EmptyQuery{}, nil
	}
	q, applicable, err := CodeQueryFromMap(ctx, m)
	if applicable {
		if err != nil {
			return nil, err
		} else {
			return q, nil
		}
	}

	q, applicable, err = PatternQueryFromMap(ctx, m)
	if applicable {
		if err != nil {
			return nil, err
		} else {
			return q, nil
		}
	}

	q, applicable, err = AndQueryFromMap(ctx, m)
	if applicable {
		if err != nil {
			return nil, err
		} else {
			return q, nil
		}
	}

	q, applicable, err = OrQueryFromMap(ctx, m)
	if applicable {
		if err != nil {
			return nil, err
		} else {
			return q, nil
		}
	}

	q, applicable, err = NotQueryFromMap(ctx, m)
	if applicable {
		if err != nil {
			return nil, err
		} else {
			return q, nil
		}
	}

	Log(WARN, ctx, "core.ParseQuery", "error", "Can't parse.", "map", m)
	return nil, NewSyntaxError("Can't handle %#v", m)
}

type GenericQuery struct {
	ctx   *Context
	query Query
}

func (q *GenericQuery) MarshalJSON() ([]byte, error) {
	return json.Marshal(&q.query)
}

func (q *GenericQuery) Get() Query {
	if q == nil {
		return nil
	}
	return q.query
}

func (q *GenericQuery) UnmarshalJSON(bs []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(bs, &m); err != nil {
		return err
	}
	query, err := ParseQuery(q.ctx, m)
	if err != nil {
		return err
	}
	q.query = query
	return nil
}

func isVariable(s string) bool {
	return strings.HasPrefix(s, "?")
}
