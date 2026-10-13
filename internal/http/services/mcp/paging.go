// Copyright 2018-2026 CERN
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

package mcp

import (
	"encoding/base64"
	"slices"
	"strings"
)

// page sorts items by key and returns at most limit of those after the cursor,
// with the cursor of the next page, empty on the last one. The cursor is the
// key of the last item returned, so that a listing that changes between pages
// neither repeats nor skips the items that stay.
func page[T any](items []T, key func(T) string, cursor string, limit int) ([]T, string, error) {
	slices.SortFunc(items, func(a, b T) int { return strings.Compare(key(a), key(b)) })
	start := 0
	if cursor != "" {
		after, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", newError(codeInvalidArgument, "cursor is not one returned by this server")
		}
		start, _ = slices.BinarySearchFunc(items, string(after), func(it T, k string) int { return strings.Compare(key(it), k) })
		for start < len(items) && key(items[start]) <= string(after) {
			start++
		}
	}
	end := min(start+limit, len(items))
	var next string
	if end < len(items) {
		next = base64.RawURLEncoding.EncodeToString([]byte(key(items[end-1])))
	}
	return items[start:end], next, nil
}

func (s *svc) pageSize(limit int) int {
	if limit <= 0 || limit > s.c.ListMaxEntries {
		return s.c.ListMaxEntries
	}
	return limit
}
