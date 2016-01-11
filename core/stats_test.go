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
	"errors"
	"testing"
)

func TestsStatsInc(t *testing.T) {
	s := &ServiceStats{}
	err := errors.New("boom")
	s.IncErrors(err)
	if s.ErrorCount != 1 {
		t.Fatalf("unexpected ErrorCount %d", s.ErrorCount)
	}
}

func TestsStatsSubtract(t *testing.T) {
	x := &ServiceStats{}
	err := errors.New("boom")
	x.IncErrors(err)
	z := x.Subtract(x)
	if z.ErrorCount != 0 {
		t.Fatalf("unexpected ErrorCount %d", z.ErrorCount)
	}
}

func TestsStatsAggregate(t *testing.T) {
	x := &ServiceStats{}
	err := errors.New("boom")
	x.IncErrors(err)
	x.Aggregate(x)
	if x.ErrorCount != 2 {
		t.Fatalf("unexpected ErrorCount %d", x.ErrorCount)
	}
}

func BenchmarkStatsClone(b *testing.B) {
	s := &ServiceStats{}
	for i := 0; i < b.N; i++ {
		s.Clone()
	}
}

func BenchmarkStatsInc(b *testing.B) {
	s := &ServiceStats{}
	err := errors.New("boom")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s.IncErrors(err)
	}

	if int(s.ErrorCount) != b.N {
		b.Fatalf("unexpected ErrorCount %d", s.ErrorCount)
	}
}
