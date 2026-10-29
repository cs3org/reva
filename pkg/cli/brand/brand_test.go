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

package brand

import (
	"slices"
	"strings"
	"testing"
)

func TestReva(t *testing.T) {
	b := Reva()
	if b.Name() != "reva" || b.ConfigDir() != "reva" || b.Connection.Transport != "grpc" || b.UI.OperatorCommands != "show" {
		t.Errorf("reva brand: %+v", b)
	}
	if got := b.Env.Names("gateway"); !slices.Equal(got, []string{"REVA_GATEWAY", "REVA_HOST"}) {
		t.Errorf("gateway env = %v", got)
	}
	for _, key := range []string{"admin_host", "token", "token_cache"} {
		if len(b.Env.Names(key)) == 0 {
			t.Errorf("no env name for %s", key)
		}
	}
	if len(b.Sum()) != 64 {
		t.Errorf("sum = %q", b.Sum())
	}
}

func TestParseIsStrict(t *testing.T) {
	for name, tc := range map[string]struct{ file, want string }{
		"unknown key":      {"[program]\nname = \"acme\"\nnmae = \"x\"\n", "unknown key program.nmae"},
		"unknown section":  {"[program]\nname = \"acme\"\n[colors]\nred = 1\n", "unknown key colors"},
		"unknown env key":  {"[program]\nname = \"acme\"\n[env]\nhost = [\"ACME_HOST\"]\n", "unknown key env.host"},
		"no name":          {"[connection]\ngateway = \"gw:9142\"\n", "program.name"},
		"bad name":         {"[program]\nname = \"Acme Box\"\n", "program.name"},
		"config dir":       {"[program]\nname = \"acme\"\nconfig_dir = \"../etc\"\n", "program.config_dir"},
		"transport":        {"[program]\nname = \"acme\"\n[connection]\ntransport = \"ftp\"\n", "connection.transport"},
		"operator mode":    {"[program]\nname = \"acme\"\n[ui]\noperator_commands = \"maybe\"\n", "ui.operator_commands"},
		"output":           {"[program]\nname = \"acme\"\n[ui]\noutput = \"xml\"\n", "ui.output"},
		"chunk size":       {"[program]\nname = \"acme\"\n[transfer]\nchunk_size = \"8 MB\"\n", "transfer.chunk_size"},
		"env name twice":   {"[program]\nname = \"acme\"\n[env]\ngateway = [\"ACME\"]\nendpoint = [\"ACME\"]\n", "ACME is also in env."},
		"bad env name":     {"[program]\nname = \"acme\"\n[env]\ngateway = [\"ACME-GW\"]\n", "not an env name"},
		"wrong value type": {"[program]\nname = \"acme\"\n[auth]\nchain = \"token\"\n", "auth.chain"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.file))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	b, err := Parse([]byte(`
[program]
name = "acme"
config_dir = "acme-cli"
[auth]
chain = ["token", "kerberos"]
kerberos = { mode = "auto", path = "/graph/v1.0/me" }
types = { kerberos = "spnego" }
[env]
endpoint = ["ACME_ENDPOINT"]
[transfer]
chunk_size = "8M"
archive = true
[messages]
network_hint = "check the VPN"
[ui]
hide = ["fleet"]
operator_commands = "auto"
`))
	if err != nil {
		t.Fatal(err)
	}
	if b.ConfigDir() != "acme-cli" || b.Auth.Types["kerberos"] != "spnego" || !*b.Transfer.Archive ||
		b.Messages.NetworkHint != "check the VPN" || b.Env.Names("endpoint")[0] != "ACME_ENDPOINT" {
		t.Errorf("brand: %+v", b)
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	MustParse([]byte("[program]\nname = \"\"\n"))
}
