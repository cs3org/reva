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

package auth

import (
	"testing"

	credentialregistry "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/registry"
	tokenregistry "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/registry"
	"github.com/cs3org/reva/v3/pkg/auth"

	// Register the strategies a deployment can name, so that
	// TestEveryStrategyIsDescribed sees all of them.
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/loader"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/loader"
)

func names(schemes []auth.Scheme) []string {
	out := make([]string, 0, len(schemes))
	for _, s := range schemes {
		out = append(out, s.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// What a deployment accepts is what it configured, in the order it tries it.
func TestSchemesFollowTheConfiguredChain(t *testing.T) {
	schemes, err := Schemes(map[string]any{
		"credential_chain": []any{"appauth", "publicshares", "basic"},
	})
	if err != nil {
		t.Fatalf("Schemes returned error: %v", err)
	}

	want := []string{"appauth", "publicshares", "basic", "revaToken"}
	if got := names(schemes); !equal(got, want) {
		t.Errorf("got schemes %v, want the chain as configured plus the token: %v", got, want)
	}

	// The strategy's own wire format comes with it, not from the chain.
	for _, s := range schemes {
		switch s.Name {
		case "appauth", "basic":
			if s.Kind != auth.SchemeBasic {
				t.Errorf("%s: got kind %q, want basic auth", s.Name, s.Kind)
			}
		case "publicshares":
			if s.Kind != auth.SchemeHeader || s.Param != "public-token" {
				t.Errorf("%s: got %q in %q, want the public-token header", s.Name, s.Kind, s.Param)
			}
		case "revaToken":
			if s.Kind != auth.SchemeHeader || s.Param != "x-access-token" {
				t.Errorf("%s: got %q in %q, want the x-access-token header", s.Name, s.Kind, s.Param)
			}
		}
		if s.Description == "" {
			t.Errorf("%s: no description, which is what a client reads", s.Name)
		}
	}
}

// A configuration that leaves the chains out is described by the defaults the
// middleware itself falls back to.
func TestSchemesResolveTheSameDefaults(t *testing.T) {
	schemes, err := Schemes(nil)
	if err != nil {
		t.Fatalf("Schemes returned error: %v", err)
	}

	want := []string{"basic", "bearer", "publicshares", "revaToken"}
	if got := names(schemes); !equal(got, want) {
		t.Errorf("got schemes %v, want the middleware's own defaults: %v", got, want)
	}
}

// A strategy nobody described would be missing from every document a
// deployment publishes, which is how this went wrong before: adding a
// strategy is also adding its description.
func TestEveryStrategyIsDescribed(t *testing.T) {
	// The loaders above are what populates the registries; without them this
	// would pass by having nothing to check.
	if len(credentialregistry.NewCredentialFuncs) == 0 || len(tokenregistry.NewTokenFuncs) == 0 {
		t.Fatal("no strategies are registered, so nothing here is being checked")
	}

	for name := range credentialregistry.NewCredentialFuncs {
		if _, ok := credentialSchemes[name]; !ok {
			t.Errorf("credential strategy %q has no description in credentialSchemes", name)
		}
	}
	for name := range tokenregistry.NewTokenFuncs {
		if _, ok := tokenSchemes[name]; !ok {
			t.Errorf("token strategy %q has no description in tokenSchemes", name)
		}
	}
}

// Two schemes cannot share a name: they are the keys a document publishes.
func TestSchemeNamesAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for strategy, s := range credentialSchemes {
		seen[s.Name] = "credential " + strategy
	}
	for strategy, s := range tokenSchemes {
		if other, ok := seen[s.Name]; ok {
			t.Errorf("token strategy %q and %s both publish the name %q", strategy, other, s.Name)
		}
		seen[s.Name] = "token " + strategy
	}
}
