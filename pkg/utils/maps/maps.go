// Copyright 2018-2023 CERN
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

package maps

// Merge returns a map containing the keys and values from both maps.
// If the two maps share a set of keys, the result map will contain
// only the value of the second map.
func Merge[K comparable, T any](m, n map[K]T) map[K]T {
	r := make(map[K]T, len(m)+len(n))
	for k, v := range m {
		r[k] = v
	}
	for k, v := range n {
		r[k] = v
	}
	return r
}

// MapValues returns a map with vales mapped using the function f.
func MapValues[K comparable, T, V any](m map[K]T, f func(T) V) map[K]V {
	r := make(map[K]V, len(m))
	for k, v := range m {
		r[k] = f(v)
	}
	return r
}

func Keys[K comparable, V any](m map[K]V) []K {
	l := make([]K, 0, len(m))
	for k := range m {
		l = append(l, k)
	}
	return l
}
