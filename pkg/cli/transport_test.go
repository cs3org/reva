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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
	"github.com/spf13/pflag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		transport, gateway, endpoint string
		want, wantErr                string
	}{
		{"", "", "", backend.GRPC, ""},
		{"grpc", "", "", backend.GRPC, ""},
		{"auto", "gw:9142", "", backend.GRPC, ""},
		{"auto", "", "", "", "no gateway and no endpoint are set"},
		{"auto", "", "https://box.example.org", backend.HTTP, ""},
		{"http", "gw:9142", "", "", "endpoint not set"},
		{"http", "gw:9142", "https://box.example.org", backend.HTTP, ""},
		{"http", "", "http://box.example.org", "", "has no TLS"},
		{"bogus", "", "", "", "unknown transport"},
	} {
		r := &Runtime{transport: tc.transport, gatewayAddr: tc.gateway, httpEndpoint: tc.endpoint}
		b, err := r.Backend(context.Background())
		switch {
		case tc.wantErr == "" && (err != nil || b.Transport() != tc.want):
			t.Errorf("%q, gateway %q: %v, %v: want the %s backend", tc.transport, tc.gateway, b, err, tc.want)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%q, gateway %q: err = %v, want %q", tc.transport, tc.gateway, err, tc.wantErr)
		}
	}
}

func TestLoadTransport(t *testing.T) {
	dir := setup(t)
	writeFile(t, filepath.Join(dir, ".config", "reva", "config.toml"), "[connection]\ntransport = \"http\"\nendpoint = \"https://config.example.org\"\n")
	t.Setenv("REVA_TRANSPORT", "auto")
	r := NewRuntime(nil)
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	r.AddFlags(fs)
	if err := fs.Parse([]string{"--endpoint", "https://flag.example.org"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Load(fs); err != nil {
		t.Fatal(err)
	}
	if r.Transport() != "auto" || r.httpEndpoint != "https://flag.example.org" {
		t.Errorf("transport %q, endpoint %q: want auto from env and the endpoint of the flag", r.Transport(), r.httpEndpoint)
	}
}

type down struct{ *clitest.Backend }

type downFiles struct{ backend.Files }

func (down) Files() backend.Files { return downFiles{} }

func (downFiles) Stat(context.Context, *provider.Reference) (*provider.ResourceInfo, error) {
	return nil, status.Error(codes.Unavailable, "connection refused")
}

type recorder struct{ notes, warns []string }

func (r *recorder) switcher(primary, secondary backend.Backend, auto bool) *switcher {
	return &switcher{
		primary: primary,
		auto:    auto,
		http:    func() (backend.Backend, error) { return secondary, nil },
		note:    func(f string, a ...any) { r.notes = append(r.notes, fmt.Sprintf(f, a...)) },
		warn:    func(f string, a ...any) { r.warns = append(r.warns, fmt.Sprintf(f, a...)) },
	}
}

func TestSwitcherNotAvailable(t *testing.T) {
	https := clitest.New("alice")
	https.AddFile("/a.txt", nil)
	var rec recorder
	s := rec.switcher(clitest.New("alice"), https, false)

	if _, err := s.Spaces().Quota(context.Background(), &provider.Reference{Path: "/"}); !errors.Is(err, backend.ErrNotAvailable) {
		t.Fatalf("quota: %v, want not available from the second backend too", err)
	}
	if len(rec.notes) != 1 || s.switched != nil {
		t.Errorf("notes %v, switched %v: want one note and no switch", rec.notes, s.switched)
	}
	if _, err := s.Files().Stat(context.Background(), &provider.Reference{Path: "/a.txt"}); !backend.IsNotFound(err) {
		t.Errorf("stat after the note: %v, want not found from the primary backend", err)
	}
}

func TestSwitcherUnavailable(t *testing.T) {
	https := clitest.New("alice")
	https.AddFile("/a.txt", nil)
	ref := &provider.Reference{Path: "/a.txt"}

	var rec recorder
	s := rec.switcher(down{clitest.New("alice")}, https, false)
	if _, err := s.Files().Stat(context.Background(), ref); status.Code(err) != codes.Unavailable {
		t.Errorf("grpc transport: %v, want Unavailable with no fallback", err)
	}

	s = rec.switcher(down{clitest.New("alice")}, https, true)
	for range 2 {
		if _, err := s.Files().Stat(context.Background(), ref); err != nil {
			t.Fatalf("auto: %v, want the answer of https", err)
		}
	}
	if len(rec.warns) != 1 || s.switched != backend.Backend(https) {
		t.Errorf("warns %v: want one warning, and the rest of the command on the second backend", rec.warns)
	}
}

func TestAbility(t *testing.T) {
	r := &Runtime{}
	r.UseBackend(clitest.New("alice"), nil)
	if _, err := Ability[backend.PreferenceStore](context.Background(), r, "preferences"); err == nil || err.Error() != "preferences is not available over fake" {
		t.Errorf("err = %v, want not available over fake", err)
	}
	r.httpEndpoint = "https://example.org"
	if _, err := Ability[backend.PreferenceStore](context.Background(), r, "preferences"); !errors.Is(err, backend.ErrNotAvailable) || err.Error() != "preferences is not available over http" {
		t.Errorf("err = %v, want not available over http either", err)
	}
}
