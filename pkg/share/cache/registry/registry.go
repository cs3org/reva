// Copyright 2018-2024 CERN
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
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package registry

import "fmt"

type CacheFunc[T any] func(map[string]interface{}) (T, error)

var registry = map[string]any{}

func Register[T any](name string, f CacheFunc[T]) {
	registry[name] = f
}

func GetCacheFunc[T any](name string) (CacheFunc[T], error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("driver not found: %s", name)
	}
	cf, ok := f.(CacheFunc[T])
	if !ok {
		return nil, fmt.Errorf("driver found but type mismatch for %s", name)
	}
	return cf, nil
}
