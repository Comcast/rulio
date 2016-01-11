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
	"io"
)

type NoopLogger struct {
}

func (l *NoopLogger) Log(level LogLevel, args ...interface{}) {
}

type SimpleLogger struct {
	w io.Writer
}

func NewSimpleLogger(w io.Writer) *SimpleLogger {
	return &SimpleLogger{w}
}

// Log implements part of the Logger interface.
func (sl *SimpleLogger) Log(level LogLevel, args ...interface{}) {
	m := make(map[string]interface{}, 20)
	m["level"] = level
	for i := 0; i < len(args); i += 2 {
		p, ok := args[i].(string)
		if !ok {
			// Should warn.
			p = Gorep(p)
		}
		if len(args) <= i+1 {
			// Should warn.
			m[p] = "missing"
		} else {
			m[p] = args[i+1]
		}
	}
	bs, err := json.Marshal(&m)
	if err != nil {
		fmt.Printf("%v\n", m)
	}
	if sl.w != nil {
		fmt.Fprintln(sl.w, string(bs))
	}
}

// Metric implements part of the Logger interface.
func (sl *SimpleLogger) Metric(name string, args ...interface{}) {
	sl.Log(METRIC, args...)
}
