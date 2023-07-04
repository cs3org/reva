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

package cfg

import (
	"github.com/go-playground/validator/v10"
	"github.com/mitchellh/mapstructure"
)

// Setter is the interface a configuration struct may implement
// to set the default options.
type Setter interface {
	// ApplyDefaults applies the default options.
	ApplyDefaults()
}

var validate = validator.New()

// Decode decodes the given raw input interface to the target pointer c.
// It applies the default configuration if the target struct
// implements the Setter interface.
// It also perform a validation to all the fields of the configuration.
func Decode(input map[string]any, c any) error {
	config := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   c,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}
	if s, ok := c.(Setter); ok {
		s.ApplyDefaults()
	}
	if err := decoder.Decode(input); err != nil {
		return err
	}

	return validate.Struct(c)
}
