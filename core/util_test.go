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
	"io"
	"strings"
	"sync"
	"testing"
)

func fromJSON(s string) map[string]interface{} {
	got, err := ParseJSON(nil, []byte(s))
	if err != nil {
		Log(ERROR, nil, "core.FromJSON", "error", err, "json", s)
		panic(err)
	}
	return got
}

func toJSON(m map[string]interface{}) string {
	js, err := json.Marshal(m)
	if err != nil {
		Log(ERROR, nil, "core.ToJSON", "error", err, "map", m)
		panic(err)
	}
	return string(js)
}

// MapJS makes a Map from JSON.
//
// If it fails, it panics, so don't use outside of tests.
func mapJS(js string) Map {
	return fromJSON(js)
}

var testJSONEvent = []byte(`
{"likes":["tacos"],
 "when":"every day",
 "with":["beer","chips"],
 "at":{"name":"home","lat":32,"lon":-97},
 "reportedBy":{"name":"Homer",
               "id":"shdhdd073ys7",
               "gender":"male",
               "smart":true}}
`)

func BenchmarkJSONEventMarshal(b *testing.B) {
	var x interface{}
	if err := json.Unmarshal(testJSONEvent, &x); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(&x); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONEventUnmarshal(b *testing.B) {
	var x interface{}
	for i := 0; i < b.N; i++ {
		if err := json.Unmarshal(testJSONEvent, &x); err != nil {
			b.Fatal(err)
		}
	}
}

const testJSON = `
{"location":"here",
  "rule": {"when":{"pattern":{"arrived":"?place"}},
           "condition":{"pattern":{"likes":"?x"}},
           "action":{"code":"console.log('offer ' + x + ' at ' + place);"}}}
	`

func BenchmarkJSONEncodingDecoder(b *testing.B) {
	const js = testJSON
	m := make(map[string]interface{})
	for n := 0; n < b.N; n++ {
		dec := json.NewDecoder(strings.NewReader(js))
		if err := dec.Decode(&m); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONEncodingEncoder(b *testing.B) {
	const js = testJSON
	m := make(map[string]interface{})
	dec := json.NewDecoder(strings.NewReader(js))
	if err := dec.Decode(&m); err != nil && err != io.EOF {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		var buf bytes.Buffer
		dec := json.NewEncoder(&buf)
		if err := dec.Encode(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONEncodingUnmarshal(b *testing.B) {
	js := []byte(testJSON)
	m := make(map[string]interface{})
	for n := 0; n < b.N; n++ {
		if err := json.Unmarshal(js, &m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONEncodingMarshal(b *testing.B) {
	js := []byte(testJSON)
	m := make(map[string]interface{})
	if err := json.Unmarshal(js, &m); err != nil {
		b.Fatal(err)
	}
	for n := 0; n < b.N; n++ {
		if _, err := json.Marshal(&m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDeferWith(b *testing.B) {
	// Does defers.

	var i = 0

	w := sync.WaitGroup{}
	w.Add(b.N)
	d := func() {
		i++
		w.Done()
	}

	f := func() {
		defer d()
	}

	for n := 0; n < b.N; n++ {
		f()
	}
	w.Wait()
}

func BenchmarkDeferWithout(b *testing.B) {
	// Doesn't do defers.

	var i = 0

	w := sync.WaitGroup{}
	w.Add(b.N)
	d := func() {
		i++
		w.Done()
	}

	f := func() {
		d()
	}

	for n := 0; n < b.N; n++ {
		f()
	}
	w.Wait()
}

func BenchmarkMutex(b *testing.B) {

	x := struct {
		sync.Mutex
		i int
	}{}

	for n := 0; n < b.N; n++ {
		x.Lock()
		x.i++
		x.Unlock()
	}
}

func BenchmarkRWMutex(b *testing.B) {

	x := struct {
		sync.RWMutex
		i int
	}{}

	for n := 0; n < b.N; n++ {
		x.RLock()
		x.i++
		x.RUnlock()
	}
}

func BenchmarkLoop(b *testing.B) {
	var i = 0

	d := func() {
		i++
	}

	f := func() {
		d()
	}

	for n := 0; n < b.N; n++ {
		f()
	}
}

var TestEvents1 = []string{`
    {
        "eventName": "exitDelay",
        "mediaType": "event/armDisarm",
        "channel": "B",
        "instance": null,
        "eventId": 401783,
        "timestamp": 1375883100160,
        "value": false,
        "properties": {
            "exitDelay": 90,
            "armLocation": "server",
            "eventTime": "2013-08-07T06:44:21-0700",
            "user": "Master"
        }
    }`,
	`{
        "eventName": "",
        "mediaType": "event/cameraAccess",
        "channel": "",
        "instanceId": "122299.2.0",
        "eventId": 846257,
        "timestamp": 1381850021888,
        "properties": {
            "internalImageUrl": "http://172.16.12.152/img/snapshot.cgi?size=2&quality=3",
            "proxyAuth": null,
            "time": 1381850015650,
            "status": "started",
            "imageUrl": "http://icqfYOR6:B5FfaJZI@76.26.112.186:47652/img/snapshot.cgi?size=2&quality=3",
            "videoUrl": "http://icqfYOR6:B5FfaJZI@76.26.112.186:47652/img/video.mjpeg",
            "proxyUrl": "/cameraProxy/proxy",
            "internalVideoUrl": "http://172.16.12.152/img/video.mjpeg",
            "label": "My Camera 1",
            "eventTime": "2013-10-15T11:13:35-0400",
            "ainfo": "aWNxZllPUjY6QjVGZmFKWkk=",
            "timeout": 300
        }
    }`,
	`{
        "eventName": "",
        "mediaType": "event/securityStateChange",
        "channel": "",
        "instance": null,
        "eventId": null,
        "timestamp": 1375883100160,
        "value": false,
        "properties": {
            "trouble": false,
            "status": "arming",
            "exitDelay": 90,
            "armType": "stay",
            "eventTime": "2013-08-07T06:44:21-0700"
        }
    }`}

func BenchmarkJSONUnmarshalEvent(b *testing.B) {

	bss := make([][]byte, 0, len(TestEvents1))
	for _, event := range TestEvents1 {
		bss = append(bss, []byte(event))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, bs := range bss {
			o := make(map[string]interface{})
			err := json.Unmarshal(bs, &o)
			if err != nil {
				b.Error(err)
			}
		}
	}
}

func BenchmarkUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		UUID()
	}
}

func TestLogAccumulatorSome(t *testing.T) {
	limit := 10
	n := 73
	a := NewAccumulator(limit)
	for i := 0; i < n; i++ {
		a.Add("chips")
	}
	if a.Dumped != n-limit {
		t.Fatalf("expected %d not %d", n-limit, a.Dumped)
	}
}

func TestLogAccumulatorZero(t *testing.T) {
	a := NewAccumulator(0)
	n := 7
	for i := 0; i < n; i++ {
		a.Add("tacos")
	}
	if a.Dumped != n {
		t.Fatalf("expected %d not %d", n, a.Dumped)
	}
}

func BenchmarkAccumulator(b *testing.B) {
	acc := NewAccumulator(100)
	chips := "chips"
	for i := 0; i < b.N; i++ {
		acc.Add(chips)
	}
}

func BenchmarkIncCounter(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IncCounter()
	}
}

func BenchmarkNowString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NowString()
	}
}

func TestStructToMap(t *testing.T) {
	x := DefaultParameters()
	m, err := StructToMap(x)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) == 0 {
		t.Fatalf("unexpected length %d", len(m))
	}
}

func TestCoerceFakeFloats(t *testing.T) {
	{
		n := 42
		x := CoerceFakeFloats(n)
		if _, ok := x.(int); !ok {
			t.Fatalf("%#v not coerced to an int (%T)", n, x)
		}
	}

	{
		n := 1439476057719
		x := CoerceFakeFloats(n)
		if _, ok := x.(int); !ok {
			t.Fatalf("%#v not coerced to an int (%T)", n, x)
		}
	}

	{
		n := 1.23e+12
		x := CoerceFakeFloats(n)
		if _, ok := x.(int); ok {
			t.Fatalf("%#v incorrectly coerced to an int", n)
		}
	}

	{
		n := 42
		m := make(map[string]interface{})
		m["foo"] = n
		x := CoerceFakeFloats(m)
		m2, ok := x.(map[string]interface{})
		if !ok {
			t.Fatalf("%#v isn't a %T", x, m)
		}
		x = m2["foo"]
		if _, ok := x.(int); ok {
			t.Fatalf("%#v not coerced to an int (%T)", n, x)
		}
	}

	{
		n := 1.23e+12
		m := make(map[string]interface{})
		m["foo"] = n
		x := CoerceFakeFloats(m)
		m2, ok := x.(map[string]interface{})
		if !ok {
			t.Fatalf("%#v isn't a %T", x, m)
		}
		x = m2["foo"]
		if _, ok := x.(int); ok {
			t.Fatalf("%#v incorrectly coerced to an int", n)
		}
	}
}

func TestIpAddresses(t *testing.T) {
	as, err := IpAddresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range as {
		fmt.Println(a)
	}
}

func TestAnIpAddresses(t *testing.T) {
	fmt.Println(AnIpAddress)
	if AnIpAddress == "" {
		t.Fatal("AnIpAddress is empty")
	}
}

func TestUseCores(t *testing.T) {
	// Not a very thorough test!
	UseCores(nil, false)
}

func TestWho(t *testing.T) {
	fmt.Println(Who(0))
	fmt.Println(Who(1))
	fmt.Println(Who(2))
}
