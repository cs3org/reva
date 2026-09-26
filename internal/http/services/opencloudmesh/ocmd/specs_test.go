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
	"reflect"
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

func validCodeFlowWebapp() *Webapp {
	return &Webapp{
		URI:          "https://app.example/hub",
		SharedSecret: "secret",
		Permissions:  []string{"read"},
		Requirements: []string{"must-exchange-token"},
		Targets:      []string{"blank"},
		AppName:      "",
		AppIconHint:  "https://app.example/icon.png",
		MediaTypes:   []string{"text/plain"},
	}
}

func TestValidateReceivedWebapp(t *testing.T) {
	blank := []string{"blank"}
	tests := []struct {
		name      string
		webapp    *Webapp
		receiver  []string
		wantError string
	}{
		{name: "compatible code flow", webapp: validCodeFlowWebapp(), receiver: blank},
		{
			name: "empty app name stays optional",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.AppName = ""
				return w
			}(),
			receiver: blank,
		},
		{
			name: "padded app name stays exact",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.AppName = " Jupyter "
				return w
			}(),
			receiver: blank,
		},
		{
			name:      "empty targets",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.Targets = nil; return w }(),
			receiver:  blank,
			wantError: "missing targets",
		},
		{
			name:      "no intersection",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.Targets = []string{"blank"}; return w }(),
			receiver:  nil,
			wantError: "no compatible target",
		},
		{
			name:      "unsupported target only",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.Targets = []string{"iframe"}; return w }(),
			receiver:  []string{"iframe"},
			wantError: "no compatible target",
		},
		{
			name: "mixed offered targets stay intact",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.Targets = []string{"iframe", "blank"}
				return w
			}(),
			receiver: blank,
		},
		{
			name:      "blank uri",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.URI = "  "; return w }(),
			receiver:  blank,
			wantError: "missing uri",
		},
		{
			name:      "malformed uri",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.URI = "http://http://evil.example/hub"; return w }(),
			receiver:  blank,
			wantError: "malformed",
		},
		{
			name:      "blank secret",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.SharedSecret = " "; return w }(),
			receiver:  blank,
			wantError: "sharedSecret",
		},
		{
			name: "malformed requirement",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.Requirements = []string{" must-exchange-token"}
				return w
			}(),
			receiver:  blank,
			wantError: "malformed requirement",
		},
		{
			name:      "absent must-exchange-token",
			webapp:    func() *Webapp { w := validCodeFlowWebapp(); w.Requirements = []string{"must-use-mfa"}; return w }(),
			receiver:  blank,
			wantError: "must-exchange-token",
		},
		{
			name: "unknown must requirement",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.Requirements = []string{"must-exchange-token", "must-sign"}
				return w
			}(),
			receiver:  blank,
			wantError: "unsupported requirement",
		},
		{
			name: "must-use-mfa",
			webapp: func() *Webapp {
				w := validCodeFlowWebapp()
				w.Requirements = []string{"must-exchange-token", "must-use-mfa"}
				return w
			}(),
			receiver:  blank,
			wantError: "must-use-mfa",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := cloneWebapp(tt.webapp)
			err := tt.webapp.ValidateReceived(tt.receiver)
			if !reflect.DeepEqual(tt.webapp, before) {
				t.Fatalf("validation mutated the DTO: got %#v want %#v", tt.webapp, before)
			}
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestWebappValidatorsAgree(t *testing.T) {
	blank := []string{"blank"}
	tests := []struct {
		name string
		reqs []string
		want string
		mfa  bool
		ok   bool
	}{
		{name: "exchange token", reqs: []string{"must-exchange-token"}, ok: true},
		{name: "padded requirement", reqs: []string{" must-exchange-token"}, want: "malformed requirement"},
		{name: "blank requirement", reqs: []string{" "}, want: "malformed requirement"},
		{name: "unknown requirement", reqs: []string{"must-exchange-token", "must-sign"}, want: "unsupported requirement"},
		{name: "missing exchange", reqs: []string{"must-use-mfa"}, want: "must-exchange-token"},
		{name: "permanent mfa", reqs: []string{"must-exchange-token", "must-use-mfa"}, mfa: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := validCodeFlowWebapp()
			offer.Requirements = tt.reqs
			received := offer.ValidateReceived(blank)
			validated := Protocols{offer}.Validate()
			if tt.ok {
				if received != nil || validated != nil {
					t.Fatalf("received %v validate %v", received, validated)
				}
				return
			}
			if tt.mfa {
				if !errors.Is(received, ErrWebappMFAUnproven) || received.Error() != ErrWebappMFAUnproven.Error() {
					t.Fatalf("received %v", received)
				}
				if !errors.Is(validated, ErrWebappMFAUnproven) || validated.Error() != ErrWebappMFAUnproven.Error() {
					t.Fatalf("validate %v", validated)
				}
				return
			}
			if received == nil || validated == nil || !strings.Contains(received.Error(), tt.want) || !strings.Contains(validated.Error(), tt.want) {
				t.Fatalf("received %v validate %v", received, validated)
			}
		})
	}
}

func TestProtocolsValidateTypedNilWebapp(t *testing.T) {
	err := Protocols{(*Webapp)(nil)}.Validate()
	if err == nil || !strings.Contains(err.Error(), "nil webapp") {
		t.Fatalf("err %v", err)
	}
}

func TestValidateAbsoluteWebappURISyntax(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "absolute https", raw: "https://app.example/hub?x=1#y", want: "https://app.example/hub?x=1#y"},
		{name: "absolute http", raw: "http://app.example/hub", want: "http://app.example/hub"},
		{name: "relative", raw: "/hub", wantErr: "invalid uri"},
		{name: "network path", raw: "//evil.example/hub", wantErr: "invalid uri"},
		{name: "userinfo", raw: "https://user:pass@app.example/hub", wantErr: "invalid uri"},
		{name: "malformed", raw: "http://http://evil.example/hub", wantErr: "invalid uri"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAbsoluteWebappURI(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != "" {
					t.Fatalf("got %q err %v", got, err)
				}
				if strings.Contains(err.Error(), "user:pass") || strings.Contains(err.Error(), "evil.example") {
					t.Fatalf("error leaked uri material: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestScreenIncomingWebappsNilAndDuplicate(t *testing.T) {
	offer := validCodeFlowWebapp()
	if err := ScreenIncomingWebapps(Protocols{offer}, []string{"blank"}); err != nil {
		t.Fatal(err)
	}
	if err := ScreenIncomingWebapps(Protocols{nil}, []string{"blank"}); err == nil {
		t.Fatal("expected nil protocol error")
	}
	if err := ScreenIncomingWebapps(Protocols{(*Webapp)(nil)}, []string{"blank"}); err == nil {
		t.Fatal("expected nil webapp error")
	}
	if err := ScreenIncomingWebapps(Protocols{offer, offer}, []string{"blank"}); err == nil {
		t.Fatal("expected ambiguous webapp error")
	}
}

func TestCloneWebappPreservesNilSlices(t *testing.T) {
	nilWebapp := &Webapp{
		URI:          "https://app.example/hub",
		SharedSecret: "secret",
	}
	copiedNil := cloneWebapp(nilWebapp)
	if copiedNil.Permissions != nil || copiedNil.Requirements != nil || copiedNil.Targets != nil || copiedNil.MediaTypes != nil {
		t.Fatalf("nil slices became %#v", copiedNil)
	}
	copiedNil.Permissions = append(copiedNil.Permissions, "read")
	if nilWebapp.Permissions != nil {
		t.Fatal("nil permission copy aliased the original")
	}

	filled := validCodeFlowWebapp()
	copied := cloneWebapp(filled)
	copied.Permissions[0] = "write"
	copied.Requirements[0] = "other"
	copied.Targets[0] = "iframe"
	copied.MediaTypes[0] = "text/html"
	if filled.Permissions[0] != "read" || filled.Requirements[0] != "must-exchange-token" || filled.Targets[0] != "blank" || filled.MediaTypes[0] != "text/plain" {
		t.Fatalf("non-nil slice copy aliased the original: %#v", filled)
	}
}

func cloneWebapp(w *Webapp) *Webapp {
	if w == nil {
		return nil
	}
	cloned := *w
	cloned.Permissions = cloneStrings(w.Permissions)
	cloned.Requirements = cloneStrings(w.Requirements)
	cloned.Targets = cloneStrings(w.Targets)
	cloned.MediaTypes = cloneStrings(w.MediaTypes)
	return &cloned
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string{}, in...)
}
