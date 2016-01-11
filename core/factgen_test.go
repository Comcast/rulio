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
	"testing"
)

func GenFactExample(t *testing.T) {
	ctx := NewContext("factgentest")

	for _, f := range genFacts(ctx, 10, 4, 3, 2, true) {
		js, err := json.MarshalIndent(f, "", "  ")
		if err == nil {
			fmt.Printf("%s\n", js)
		} else {
			t.Errorf("JSON marshal error %v on %v\n", err, f)
		}
	}
}

func TestRandomFacts(t *testing.T) {
	ctx := NewContext("factgentest")

	_, err := RandomFactsTest(ctx, 10, 10, t)
	if err != nil {
		t.Error(err)
	}
}
