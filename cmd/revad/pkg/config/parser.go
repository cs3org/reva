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

package config

import (
	"io"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Command is the command to execute after parsing the template.
type Command interface{ isCommand() }

// FieldByKey instructs the template runner to get a field by a key.
type FieldByKey struct{ Key string }

func (FieldByKey) isCommand() {}

// FieldByIndex instructs the template runner to get a field by an index.
type FieldByIndex struct{ Index int }

func (FieldByIndex) isCommand() {}

func parseNext(key string) (Command, string, error) {
	// key = ".grpc.services.authprovider[1].address"

	key = strings.TrimSpace(key)

	// first character must be either "." or "["
	// unless the key is empty
	if key == "" {
		return nil, "", io.EOF
	}

	switch {
	case strings.HasPrefix(key, "."):
		tkn, next := split(key)
		return FieldByKey{Key: tkn}, next, nil
	case strings.HasPrefix(key, "["):
		tkn, next := split(key)
		index, err := strconv.ParseInt(tkn, 10, 64)
		if err != nil {
			return nil, "", errors.Wrap(err, "parsing error")
		}
		return FieldByIndex{Index: int(index)}, next, nil
	}

	return nil, "", errors.New("parsing error: operator not recognised")
}

func split(key string) (token string, next string) {
	// key = ".grpc.services.authprovider[1].address"
	//         -> grpc
	// key = "[<i>].address"
	// 		   -> <i>
	if key == "" {
		return
	}

	i := -1
	s := key[0]
	key = key[1:]

	switch s {
	case '.':
		i = strings.IndexAny(key, ".[")
	case '[':
		i = strings.IndexByte(key, ']')
	}

	if i == -1 {
		return key, ""
	}

	if key[i] == ']' {
		return key[:i], key[i+1:]
	}
	return key[:i], key[i:]
}
