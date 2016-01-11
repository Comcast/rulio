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
	"io/ioutil"
	"testing"
)

func TestLog(t *testing.T) {
	ctx := NewContext("test")
	ctx.Verbosity = ABSURD
	Log(INFO, ctx, "TestLog", "x", 1, "y", "two", "z", struct{}{},
		"nil", nil,
		"fact", map[string]interface{}{"a": "1"})
}

func BenchmarkLogBasic(b *testing.B) {
	ctx := NewContext("test")
	ctx.Verbosity = ABSURD
	a := []interface{}{1, 2}
	s := struct{}{}
	m := map[string]interface{}{"foo": "bar"}
	for i := 0; i < b.N; i++ {
		Log(INFO, ctx, "BenchmarkLog", "one", 1, "two", "two",
			"struct", s,
			"array", a,
			"three", 3.0,
			"map", m)
	}
}

func benchmarkLogger(b *testing.B, logger Logger) {
	a := []interface{}{1, 2}
	s := struct{}{}
	m := map[string]interface{}{"foo": "bar"}
	for i := 0; i < b.N; i++ {
		logger.Log(ERROR,
			"op", "BenchmarkLogExternal", "one", 1, "two", "two",
			"struct", s,
			"array", a,
			"three", 3.0,
			"map", m)
	}
}

// Can't benchmark with a constructor that takes a writer.
// func BenchmarkLogExternal(b *testing.B) {
// 	benchmarkLogger(b, NewExternalLogger(ioutil.Discard))
// }

func BenchmarkLogSimple(b *testing.B) {
	benchmarkLogger(b, NewSimpleLogger(ioutil.Discard))
}

func TestLogParseVerbosity(t *testing.T) {
	n, err := ParseVerbosity("ERROR|SYS")
	if err != nil {
		t.Fatal(err)
	}
	if n != ERROR|SYS {
		t.Fatalf("ERROR|SYS %0x != %0x", ERROR|SYS, n)
	}
}

func TestLogAccumulatorLogging(t *testing.T) {
	DefaultLogger = NewSimpleLogger(ioutil.Discard)
	ctx := TestContext("LogAccumulator")
	ctx.LogAccumulator = NewAccumulator(10000)
	ctx.LogAccumulatorLevel = EVERYTHING
	ctx.Verbosity = EVERYTHING
	Log(INFO, ctx, "Hello")
	if n := len(ctx.LogAccumulator.Acc); n == 0 {
		t.Fatal("wanted to accumulate some logs")
	}
}

func TestLogAccumulatorLogingSpill(t *testing.T) {
	DefaultLogger = NewSimpleLogger(ioutil.Discard)
	ctx := TestContext("LogAccumulator")
	ctx.LogAccumulator = NewAccumulator(10)
	ctx.LogAccumulatorLevel = EVERYTHING
	ctx.Verbosity = EVERYTHING
	for i := 0; i < 100; i++ {
		Log(INFO, ctx, "Hello")
	}
	if n := len(ctx.LogAccumulator.Acc); n == 0 {
		t.Fatal("wanted to accumulate some logs")
	}
	if ctx.LogAccumulator.Dumped == 0 {
		t.Fatal("wanted to have spilled some logs")
	}
}
