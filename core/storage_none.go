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

// Implementation of Storage that does nothing at all.

type NoStorage struct {
}

func NewNoStorage(ctx *Context) (*NoStorage, error) {
	return &(NoStorage{}), nil
}

func (s *NoStorage) Load(ctx *Context, loc string) ([]Pair, error) {
	return nil, nil
}

func (s *NoStorage) Add(ctx *Context, loc string, m *Pair) error {
	return nil
}

func (s *NoStorage) Remove(ctx *Context, loc string, k []byte) (int64, error) {
	return 0, nil
}

func (s *NoStorage) Clear(ctx *Context, loc string) (int64, error) {
	return 0, nil
}

func (s *NoStorage) Delete(ctx *Context, loc string) error {
	return nil
}

func (s *NoStorage) GetStats(ctx *Context, loc string) (StorageStats, error) {
	return StorageStats{}, nil
}

func (s *NoStorage) Close(ctx *Context) error {
	return nil
}

func (s *NoStorage) Health(ctx *Context) error {
	return nil
}
