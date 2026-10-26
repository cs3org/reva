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

package cli

import (
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/state"
	"github.com/spf13/pflag"
)

func TestPluginConfig(t *testing.T) {
	setup(t)
	if err := state.Write("config.toml", []byte("[plugins.otg]\nnumber = 42\n[plugins.typo]\nnumbr = 1\n")); err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{}
	fs := pflag.NewFlagSet("reva", pflag.ContinueOnError)
	rt.AddFlags(fs)
	if err := rt.Load(fs); err != nil {
		t.Fatal(err)
	}

	var c struct{ Number int }
	if err := PluginConfig(rt, "otg", &c); err != nil || c.Number != 42 {
		t.Errorf("number = %d, err = %v: want 42", c.Number, err)
	}
	if err := PluginConfig(rt, "typo", &c); err == nil || !strings.Contains(err.Error(), "numbr") {
		t.Errorf("err = %v: want the unknown key numbr", err)
	}
	c.Number = 7
	if err := PluginConfig(rt, "missing", &c); err != nil || c.Number != 7 {
		t.Errorf("a missing section changed the value to %d (err %v)", c.Number, err)
	}
}
