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
)

type Problem interface {
	IsFatal() bool
	Error() string
}

type Condition struct {
	Msg string `json:"msg,omitempty"`
	// Hope should be either "fatal", "unknown", or something else ...
	// Can make enums when we understand usage.

	// There are times in life when you should ask
	// questions. There are times in life when you shouldn't. When
	// you see the EOD tech running up the flight deck, the latter
	// ruler applies.
	//
	//   --Jim Doran, Air Gunner, USN (Ret)
	//
	// Note: An "EOD tech" is an "Explosive Ordnance Disposal Technician"

	Hope string `json:"status,omitempty"`
}

func (c *Condition) Error() string {
	if c == nil {
		return "nil condition"
	}
	return c.Msg
}

func (c *Condition) IsFatal() bool {
	return c.Hope == "fatal"
}

func (c *Condition) String() string {
	return "Condition: " + c.Msg + " (hope: " + c.Hope + ")"
}

type ExpiredError struct {
}

func (e *ExpiredError) Error() string {
	return "expired"
}

func (e *ExpiredError) IsFatal() bool {
	return true // I guess.
}

func (e *ExpiredError) String() string {
	return "Expired"
}

type SyntaxError struct {
	Msg string
}

func NewSyntaxError(s string, args ...interface{}) *SyntaxError {
	return &SyntaxError{fmt.Sprintf(s, args...)}
}

func (e *SyntaxError) Error() string {
	return e.Msg
}

func (e *SyntaxError) IsFatal() bool {
	return true
}

func (e *SyntaxError) String() string {
	return "SyntaxError: " + e.Msg
}

type NotFoundError struct {
	Msg string
}

func NewNotFoundError(s string, args ...interface{}) *NotFoundError {
	return &NotFoundError{fmt.Sprintf(s, args...)}
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Msg
}

func (e *NotFoundError) IsFatal() bool {
	// I guess.
	return false
}

func (e *NotFoundError) String() string {
	return "NotFoundError: " + e.Msg
}
