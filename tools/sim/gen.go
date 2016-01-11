package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/Comcast/rulio/core"
)

var Speed = float64(50.0)

type ShortId struct {
}

func (d ShortId) Emit() (interface{}, error) {
	id := fmt.Sprintf("%016x", rand.Int63())
	return id, nil
}

type UUID struct {
}

func (d UUID) Emit() (interface{}, error) {
	return core.UUID(), nil
}

type TimestampMillis struct {
}

func (d TimestampMillis) Emit() (interface{}, error) {
	return time.Now().Nanosecond() / 1000000, nil
}

type Normal struct {
	Mean   float64
	Stddev float64
	Min    float64
}

func (d Normal) Emit() (interface{}, error) {
	x := d.Mean + rand.NormFloat64()*d.Stddev
	if x < d.Min {
		x = d.Min
	}
	return x, nil
}

type Zipf struct {
	S    float64
	V    float64
	Imax uint64
	zipf *rand.Zipf
}

func (d Zipf) Emit() (interface{}, error) {
	if d.zipf == nil {
		r := rand.New(rand.NewSource(42))
		d.zipf = rand.NewZipf(r, d.S, d.V, d.Imax)
	}
	return d.zipf.Uint64(), nil
}

type Uniform struct {
	Min float64
	Max float64
}

func (d Uniform) Emit() (interface{}, error) {
	if d.Max <= d.Min {
		return 0.0, fmt.Errorf("%f <= %f", d.Max, d.Min)
	}
	return d.Min + (d.Max-d.Min)*rand.Float64(), nil
}

type Choose map[string]float64

func (d Choose) Emit() (interface{}, error) {
	bins := make([]float64, len(d))
	choices := make([]string, len(d))
	total := 0.0
	for _, v := range d {
		total += v
	}

	acc := 0.0
	i := 0
	for p, v := range d {
		acc += v / total
		bins[i] = acc
		choices[i] = p
		i++
	}
	p := rand.Float64()
	acc = 0.0
	for i = 0; i < len(bins); i++ {
		if p < bins[i] {
			return choices[i], nil
		}
	}
	panic("busted Choose.Emit()")
	return nil, nil
}

type Generate struct {
	Uniform         *Uniform
	Choose          Choose
	Normal          *Normal
	Zipf            *Zipf
	TimestampMillis *TimestampMillis
	ShortId         *ShortId
	UUID            *UUID
}

func NewGen(x interface{}) (*Generate, error) {
	js, err := json.Marshal(&x)
	if err != nil {
		return nil, err
	}
	var gen Generate
	err = json.Unmarshal(js, &gen)
	if err != nil {
		return nil, err
	}
	return &gen, nil
}

func (g Generate) Emit() (interface{}, error) {
	if g.Uniform != nil {
		return g.Uniform.Emit()
	}
	if g.Choose != nil {
		return g.Choose.Emit()
	}
	if g.Normal != nil {
		return g.Normal.Emit()
	}
	if g.Zipf != nil {
		return g.Zipf.Emit()
	}
	if g.TimestampMillis != nil {
		return g.TimestampMillis.Emit()
	}
	if g.ShortId != nil {
		return g.ShortId.Emit()
	}
	if g.UUID != nil {
		return g.UUID.Emit()
	}
	return nil, fmt.Errorf("no distribution")
}

type Frequency struct {
	Generate Generate
	Duration string
	Secs     int
}

func (f *Frequency) Next() (time.Duration, error) {
	if 0 < f.Secs {
		d := time.Duration(int64(1000000000.0 * float64(f.Secs) / Speed))
		return d, nil
	}

	if f.Duration != "" {
		return time.ParseDuration(f.Duration)
	}

	never := 0 * time.Second

	x, err := f.Generate.Emit()
	if err != nil {
		return never, err
	}
	n, okay := x.(float64)
	if !okay {
		return never, fmt.Errorf("didn't expect a %T (%#v)", x, x)
	}

	return time.Duration(int64(1000000000.0 * n / Speed)), nil
}

type Event map[string]interface{}

type EventSpec struct {
	Event     map[string]interface{}
	Frequency Frequency
}

func GenWalk(e map[string]interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{})
	for p, v := range e {
		switch vv := v.(type) {
		case Generate:
			var err error
			if v, err = vv.Emit(); err != nil {
				return nil, err
			}
		case map[string]interface{}:
			var err error
			g, have := vv["Generate"]
			if have {
				gen, err := NewGen(g)
				if err != nil {
					return nil, err
				}
				// Cache it!
				e[p] = *gen
				if v, err = gen.Emit(); err != nil {
					return nil, err
				}
			} else {
				if v, err = GenWalk(vv); err != nil {
					return nil, err
				}
			}
		}
		out[p] = v
	}
	return out, nil
}

func (s *EventSpec) Gen() (Event, error) {
	return GenWalk(s.Event)
}

func (s *EventSpec) Next() (time.Duration, error) {
	return s.Frequency.Next()
}

type DeviceType struct {
	Events map[string]EventSpec
}

func (dt *DeviceType) Emit(device string, f func(x interface{}) error) {
	for eid, e := range dt.Events {
		go func(eid string, e *EventSpec) {
			for {
				wait, err := e.Next()
				if err != nil {
					panic(err)
				}
				// log.Printf("%s %s sleeping %v", device, eid, wait)
				time.Sleep(wait)
				event, err := e.Gen()
				if err != nil {
					panic(err)
				}
				event["device"] = device
				if err = f(event); err != nil {
					panic(err)
				}
			}
		}(eid, &e)
	}

}

type DeviceTypes struct {
	Types map[string]DeviceType
}

func getJS(js string) ([]byte, error) {
	if strings.HasSuffix(js, ".js") {
		filename := js
		log.Printf("Loading %s", filename)
		bs, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		return bs, nil
	}
	return []byte(js), nil
}

func NewDeviceTypes(js string) (*DeviceTypes, error) {
	bs, err := getJS(js)
	if err != nil {
		return nil, err
	}
	var types DeviceTypes
	if err := json.Unmarshal(bs, &types); err != nil {
		return nil, err
	}
	return &types, nil
}

type DeviceTypeName string

type Account struct {
	Name    string
	Devices map[string]DeviceTypeName
}

func (a *Account) FindDevice(deviceType string) (string, error) {
	candidates := make([]string, 0, 0)
	for device, dType := range a.Devices {
		if string(dType) == deviceType {
			candidates = append(candidates, device)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no %s available", deviceType)
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (a *Account) Pretty() string {
	js, err := json.MarshalIndent(a, "  ", "  ")
	if err != nil {
		panic(err)
	}
	return string(js)
}

func (types *DeviceTypes) GenAccount(n int) (*Account, error) {
	if n < 0 {
		n = rand.Int()
	}
	name := fmt.Sprintf("account_%d", n)
	devices := make(map[string]DeviceTypeName)
	i := 0
	for typeName, _ := range types.Types {
		device := fmt.Sprintf("%s%d", typeName, i)
		i++
		devices[device] = DeviceTypeName(typeName)
	}
	return &Account{Name: name, Devices: devices}, nil
}

func (a *Account) Emit(types *DeviceTypes, f func(req interface{}) error) {
	for device, typeName := range a.Devices {
		deviceType, have := types.Types[string(typeName)]
		if !have {
			err := fmt.Errorf("unknown device type %s", typeName)
			panic(err)
		}
		fr := func(e interface{}) error {
			req := make(map[string]interface{})
			req["location"] = a.Name
			req["uri"] = "/loc/events/ingest"
			req["event"] = e
			return f(req)
		}
		deviceType.Emit(device, fr)
	}
}

func instantiate(m map[string]interface{}, params map[string]string) (map[string]interface{}, error) {
	// Not fast but is simple.

	bs, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	js := string(bs)
	for name, val := range params {
		re, err := regexp.Compile(`\b` + name + `\b`)
		if err != nil {
			return nil, err
		}
		js = re.ReplaceAllLiteralString(js, val)
	}

	out := make(map[string]interface{})
	err = json.Unmarshal([]byte(js), &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type Rule map[string]interface{}

func (r *Rule) instantiate(params map[string]string) (*Rule, error) {
	m, err := instantiate(map[string]interface{}(*r), params)
	if err != nil {
		return nil, err
	}
	rule := Rule(m)
	return &rule, nil
}

type Fact map[string]interface{}

func (f *Fact) instantiate(params map[string]string) (*Fact, error) {
	m, err := instantiate(map[string]interface{}(*f), params)
	if err != nil {
		return nil, err
	}
	fact := Fact(m)
	return &fact, nil
}

type Template struct {
	Name        string
	Description string
	Parameters  map[string]string
	Rules       map[string]Rule
	Facts       map[string]Fact
}

func NewTemplate(js string) (*Template, error) {
	bs, err := getJS(js)
	if err != nil {
		return nil, err
	}
	var template Template
	if err := json.Unmarshal(bs, &template); err != nil {
		return nil, err
	}
	return &template, nil
}

func (t *Template) Instantiate(params map[string]string) (map[string]Rule, map[string]Fact, error) {
	rules := make(map[string]Rule)
	for rid, r := range t.Rules {
		rule, err := r.instantiate(params)
		if err != nil {
			return nil, nil, err
		}
		rules[rid] = *rule
	}
	facts := make(map[string]Fact)
	for id, f := range t.Facts {
		fact, err := f.instantiate(params)
		if err != nil {
			return nil, nil, err
		}
		facts[id] = *fact
	}
	return rules, facts, nil
}

func (a *Account) Rules(t *Template, config map[string]interface{}) (map[string]Rule, map[string]Fact, error) {
	// Need to find devices for the deviceTypes in the template's Parameters.
	params := make(map[string]string)
	for p, v := range config {
		switch s := v.(type) {
		case string:
			params[p] = s
		default:
			params[p] = fmt.Sprintf("%v", v)
		}
	}
	params["ACCOUNT"] = a.Name
	for sym, deviceType := range t.Parameters {
		device, err := a.FindDevice(deviceType)
		if err != nil {
			return nil, nil, err
		}
		log.Printf("%s (%s) %s", device, sym, deviceType)
		params[sym] = device
	}
	return t.Instantiate(params)
}
