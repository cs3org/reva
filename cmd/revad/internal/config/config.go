// Copyright 2018-2021 CERN
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
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

// Read reads the configuration from the reader.
func Read(r io.Reader) (map[string]interface{}, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		err = errors.Wrap(err, "config: error reading from reader")
		return nil, err
	}

	v := map[string]interface{}{}
	err = toml.Unmarshal(data, &v)
	if err != nil {
		err = errors.Wrap(err, "config: error decoding toml data")
		return nil, err
	}

	return v, nil
}
