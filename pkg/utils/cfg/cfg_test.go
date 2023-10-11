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

package cfg_test

import (
	"testing"

	"github.com/cs3org/reva/v2/pkg/utils/cfg"
	"github.com/stretchr/testify/assert"
)

type NoDefaults struct {
	A string `mapstructure:"a"`
	B int    `mapstructure:"b"`
	C bool   `mapstructure:"c"`
}

type WithDefaults struct {
	A string `mapstructure:"a"`
	B int    `mapstructure:"b" validate:"required"`
}

func (c *WithDefaults) ApplyDefaults() {
	if c.A == "" {
		c.A = "default"
	}
}

func TestDecode(t *testing.T) {
	t1 := map[string]any{
		"b": 10,
		"c": true,
	}
	var noDefaults NoDefaults
	if err := cfg.Decode(t1, &noDefaults); err != nil {
		t.Fatal("not expected error", err)
	}
	assert.Equal(t, NoDefaults{
		A: "",
		B: 10,
		C: true,
	}, noDefaults)

	t2 := map[string]any{
		"b": 100,
	}
	var defaults WithDefaults
	if err := cfg.Decode(t2, &defaults); err != nil {
		t.Fatal("not expected error", err)
	}
	assert.Equal(t, WithDefaults{
		A: "default",
		B: 100,
	}, defaults)

	t3 := map[string]any{
		"a": "string",
	}
	var required WithDefaults
	if err := cfg.Decode(t3, &required); err == nil {
		t.Fatal("expected error, but none returned")
	}
}
