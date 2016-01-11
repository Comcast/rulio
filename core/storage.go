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

// ToDo: Use this less
type Pair struct {
	K []byte
	V []byte
}

func (d *Pair) String() string {
	return fmt.Sprintf(`Pair{"%s","%s"}`, d.K, d.V)
}

type StorageStats struct {
	NumRecords       int
	DateOfLastRecord string
}

type Storage interface {
	// ListLocations(ctx *Context) ([]string, error)

	Load(ctx *Context, loc string) ([]Pair, error)

	Add(ctx *Context, loc string, data *Pair) error

	Remove(ctx *Context, loc string, k []byte) (int64, error)

	Clear(ctx *Context, loc string) (int64, error)

	Delete(ctx *Context, loc string) error

	GetStats(ctx *Context, loc string) (StorageStats, error)

	Close(ctx *Context) error

	Health(ctx *Context) error
}
