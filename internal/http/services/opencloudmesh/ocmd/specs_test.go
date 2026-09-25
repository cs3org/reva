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

package ocmd

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateWebappLaunch(t *testing.T) {
	blank := []string{"blank"}
	tests := []struct {
		name       string
		uri        string
		secret     string
		reqs       []string
		targets    []string
		receiver   []string
		want       string
		mfa        bool
		invalidURI bool
	}{
		{
			name:     "absolute https",
			uri:      "https://app.example/hub?x=1#y",
			secret:   "secret",
			reqs:     []string{"must-exchange-token"},
			targets:  blank,
			receiver: blank,
		},
		{
			name:     "empty app name is not required",
			uri:      "https://app.example/hub",
			secret:   "secret",
			reqs:     []string{"must-exchange-token"},
			targets:  []string{"iframe", "blank"},
			receiver: blank,
		},
		{
			name:    "missing secret",
			uri:     "https://app.example/hub",
			reqs:    []string{"must-exchange-token"},
			targets: blank, receiver: blank,
			want: "sharedSecret",
		},
		{
			name:    "padded requirement",
			uri:     "https://app.example/hub",
			secret:  "secret",
			reqs:    []string{" must-exchange-token"},
			targets: blank, receiver: blank,
			want: "malformed requirement",
		},
		{
			name:    "unknown requirement",
			uri:     "https://app.example/hub",
			secret:  "secret",
			reqs:    []string{"must-exchange-token", "must-sign"},
			targets: blank, receiver: blank,
			want: "unsupported requirement",
		},
		{
			name:    "missing token exchange",
			uri:     "https://app.example/hub",
			secret:  "secret",
			reqs:    []string{},
			targets: blank, receiver: blank,
			want: "must-exchange-token",
		},
		{
			name:     "permanent mfa rejection",
			uri:      "https://app.example/hub",
			secret:   "secret",
			reqs:     []string{"must-exchange-token", "must-use-mfa"},
			targets:  blank,
			receiver: blank,
			mfa:      true,
		},
		{
			name:    "relative uri",
			uri:     "/apps/open",
			secret:  "secret",
			reqs:    []string{"must-exchange-token"},
			targets: blank, receiver: blank,
			invalidURI: true,
		},
		{
			name:    "iframe only",
			uri:     "https://app.example/hub",
			secret:  "secret",
			reqs:    []string{"must-exchange-token"},
			targets: []string{"iframe"}, receiver: blank,
			want: "no compatible target",
		},
		{
			name:    "receiver does not advertise blank",
			uri:     "https://app.example/hub",
			secret:  "secret",
			reqs:    []string{"must-exchange-token"},
			targets: blank, receiver: []string{},
			want: "no compatible target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebappLaunch(tt.uri, tt.secret, tt.reqs, tt.targets, tt.receiver)
			if tt.mfa {
				if !errors.Is(err, ErrWebappMFAUnproven) {
					t.Fatalf("err = %v", err)
				}
				if err.Error() != ErrWebappMFAUnproven.Error() {
					t.Fatalf("sentence = %q", err.Error())
				}
				return
			}
			if tt.invalidURI {
				if !errors.Is(err, ErrInvalidProtocolURI) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestWebappToOCMProtocolPreservesDescriptor(t *testing.T) {
	w := &Webapp{
		URI:          "https://app.example/hub",
		SharedSecret: "secret",
		Permissions:  []string{"view"},
		Requirements: []string{"must-exchange-token"},
		Targets:      []string{"blank"},
		AppName:      "",
		MediaTypes:   []string{"text/plain"},
	}
	proto := w.ToOCMProtocol()
	got := proto.GetWebappOptions()
	if got == nil {
		t.Fatal("missing webapp options")
	}
	if got.Uri != w.URI || got.SharedSecret != w.SharedSecret || got.AppName != "" {
		t.Fatalf("descriptor %+v", got)
	}
	if len(got.Requirements) != 1 || got.Requirements[0] != "must-exchange-token" {
		t.Fatalf("requirements %v", got.Requirements)
	}
	if len(got.Targets) != 1 || got.Targets[0] != "blank" {
		t.Fatalf("targets %v", got.Targets)
	}
	if len(got.MediaTypes) != 1 || got.MediaTypes[0] != "text/plain" {
		t.Fatalf("media %v", got.MediaTypes)
	}
}

func TestValidateProtocolURISentinel(t *testing.T) {
	err := validateProtocolURI("webapp", "https://https://app.example/hub")
	if !errors.Is(err, ErrInvalidProtocolURI) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "https://https://") {
		t.Fatalf("sentinel leaked the uri: %v", err)
	}
	if err := validateProtocolURI("webdav", "https://dav.example/dav"); err != nil {
		t.Fatal(err)
	}
}
