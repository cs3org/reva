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

package ocmshareprovider

import (
	"bytes"
	"context"
	"strings"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

func TestFreshAccessMethodsNilAndEmpty(t *testing.T) {
	svc := &service{conf: &config{OfferWebapp: true, WebappName: testAppName}}
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := appctx.WithLogger(context.Background(), &logger)

	got, err := svc.freshAccessMethods(ctx, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("nil request methods -> %#v, want empty slice", got)
	}
	got, err = svc.freshAccessMethods(ctx, []*ocm.AccessMethod{}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty request methods -> %#v, want empty slice", got)
	}
	got, err = svc.freshAccessMethods(ctx, []*ocm.AccessMethod{nil, nil}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("nil elements produced %#v", got)
	}
	if strings.Contains(logs.String(), webappOmissionReason) {
		t.Fatalf("empty input logged an omission: %s", logs.String())
	}
}

func TestCreateOCMShareRejectsInvalidAccessMethods(t *testing.T) {
	legacy := []string{"legacy-req"}
	cases := []struct {
		name    string
		offer   bool
		methods []*ocm.AccessMethod
		disco   discoverySpec
	}{
		{
			name:  "nil webapp options",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				{Term: &ocm.AccessMethod_WebappOptions{}},
			},
			disco: capableDiscovery(),
		},
		{
			name:  "typed nil webapp options",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				{Term: (*ocm.AccessMethod_WebappOptions)(nil)},
			},
			disco: capableDiscovery(),
		},
		{
			name:    "duplicate webapps",
			offer:   true,
			methods: []*ocm.AccessMethod{webdavMethod(), webappMethod("one"), webappMethod("two")},
			disco:   capableDiscovery(),
		},
		{
			name:  "duplicate requirements",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{reqMustExchangeToken, reqMustExchangeToken}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "padded requirement",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{" " + reqMustExchangeToken}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "unknown requirement",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{reqMustExchangeToken, secretMarker}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "blank requirement",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{reqMustExchangeToken, " "}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "missing must-exchange-token",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{reqMustUseMFA}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "mixed mfa conflicts with webdav",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethodWith([]string{reqMustExchangeToken}),
				webappMethodWith("", []string{reqMustUseMFA, reqMustExchangeToken}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "mixed mfa with empty webdav",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethodWith("", []string{reqMustExchangeToken, reqMustUseMFA}),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "nil webdav options",
			offer: true,
			methods: []*ocm.AccessMethod{
				{Term: &ocm.AccessMethod_WebdavOptions{}},
				webappMethod(""),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "typed nil webdav options",
			offer: true,
			methods: []*ocm.AccessMethod{
				{Term: (*ocm.AccessMethod_WebdavOptions)(nil)},
				webappMethod(""),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "nil webdav permissions",
			offer: true,
			methods: []*ocm.AccessMethod{
				{
					Term: &ocm.AccessMethod_WebdavOptions{
						WebdavOptions: &ocm.WebDAVAccessMethod{},
					},
				},
				webappMethod(""),
			},
			disco: capableDiscovery(),
		},
		{
			name:  "webdav-only nil permissions",
			offer: true,
			methods: []*ocm.AccessMethod{
				{
					Term: &ocm.AccessMethod_WebdavOptions{
						WebdavOptions: &ocm.WebDAVAccessMethod{Requirements: legacy},
					},
				},
			},
			disco: capableDiscovery(),
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			shareFixture{
				offer:         tt.offer,
				methods:       tt.methods,
				disco:         tt.disco,
				expectInvalid: true,
				marker:        secretMarker,
				logs:          logs,
			}.run(t)
		})
	}
}

func TestCreateOCMShareIgnoresInvalidWebappWhenGateIsFalse(t *testing.T) {
	logs := &bytes.Buffer{}
	wire, stored, repo, _ := shareFixture{
		offer: false,
		methods: []*ocm.AccessMethod{
			webdavMethodWith([]string{"legacy-req"}),
			{Term: &ocm.AccessMethod_WebappOptions{WebappOptions: &ocm.WebappAccessMethod{AppName: secretMarker}}},
			{Term: (*ocm.AccessMethod_WebappOptions)(nil)},
		},
		disco:  capableDiscovery(),
		marker: secretMarker,
		logs:   logs,
	}.run(t)
	if repo.storeN != 1 {
		t.Fatalf("stores = %d, want 1", repo.storeN)
	}
	assertNoWebapp(t, wire, stored)
	if !slicesEqual(storedWebDAV(t, stored).Requirements, []string{"legacy-req"}) {
		t.Fatalf("webdav requirements = %#v", storedWebDAV(t, stored).Requirements)
	}
	if strings.Count(logs.String(), webappOmissionReason) != 1 {
		t.Fatalf("omission logs = %q", logs.String())
	}
}

func TestCreateOCMShareDefaultAndExplicitRequirements(t *testing.T) {
	t.Run("empty webapp requirements copy the default", func(t *testing.T) {
		caller := webappMethodWith("incoming", nil)
		wire, stored, _, _ := shareFixture{
			offer:   true,
			methods: []*ocm.AccessMethod{webdavMethod(), caller},
			disco:   capableDiscovery(),
		}.run(t)
		web := webapps(wire.Protocols)
		if len(web) != 1 {
			t.Fatalf("wire webapps = %d", len(web))
		}
		storedOpts := storedWebapp(t, stored)
		if !slicesEqual(storedOpts.Requirements, share.DefaultWebappRequirements) {
			t.Fatalf("stored requirements = %#v", storedOpts.Requirements)
		}
		if !slicesEqual(web[0].Requirements, share.DefaultWebappRequirements) {
			t.Fatalf("wire requirements = %#v", web[0].Requirements)
		}
		if !slicesEqual(web[0].Targets, []string{"blank"}) {
			t.Fatalf("wire targets = %#v", web[0].Targets)
		}
		if storedOpts.ProtoReflect().Descriptor().Fields().ByName("targets") != nil {
			t.Fatal("stored access method invented a targets field")
		}
		dav := storedWebDAV(t, stored)
		if !slicesEqual(dav.Requirements, share.DefaultWebappRequirements) {
			t.Fatalf("mixed empty webdav requirements = %#v", dav.Requirements)
		}
		if sameSlice(storedOpts.Requirements, dav.Requirements) || sameSlice(storedOpts.Requirements, share.DefaultWebappRequirements) {
			t.Fatal("requirement copies alias each other or the package default")
		}
		storedOpts.Requirements[0] = "mutated-webapp"
		dav.Requirements[0] = "mutated-webdav"
		web[0].Requirements[0] = "mutated-wire"
		web[0].Targets[0] = "mutated-target"
		if share.DefaultWebappRequirements[0] != reqMustExchangeToken {
			t.Fatal("mutation changed the package default requirements")
		}
		if share.DefaultWebappTargets[0] != "blank" {
			t.Fatal("mutation changed the package default targets")
		}
		if caller.GetWebappOptions().GetRequirements() != nil {
			t.Fatal("nil caller requirements were replaced")
		}
	})

	t.Run("explicit requirements keep order", func(t *testing.T) {
		reqs := []string{reqMustUseMFA, reqMustExchangeToken}
		caller := webappMethodWith("incoming", reqs)
		wire, stored, _, _ := shareFixture{
			offer:   true,
			methods: []*ocm.AccessMethod{caller},
			disco:   capableDiscovery(),
		}.run(t)
		storedOpts := storedWebapp(t, stored)
		if !slicesEqual(storedOpts.Requirements, reqs) {
			t.Fatalf("stored requirements = %#v", storedOpts.Requirements)
		}
		if !slicesEqual(webapps(wire.Protocols)[0].Requirements, reqs) {
			t.Fatalf("wire requirements = %#v", webapps(wire.Protocols)[0].Requirements)
		}
		storedOpts.Requirements[0] = "mutated"
		if reqs[0] != reqMustUseMFA {
			t.Fatal("stored requirements alias the caller slice")
		}
		if caller.GetWebappOptions().AppName != "incoming" {
			t.Fatal("caller app name was overwritten")
		}
	})
}

func TestCreateOCMShareMixedRequirementsAgree(t *testing.T) {
	callerReqs := []string{reqMustExchangeToken}
	caller := webappMethodWith("incoming", callerReqs)
	davCaller := webdavMethodWith([]string{reqMustExchangeToken})
	wire, stored, _, _ := shareFixture{
		offer:   true,
		methods: []*ocm.AccessMethod{davCaller, caller},
		disco:   capableDiscovery(),
	}.run(t)
	if !slicesEqual(webdavs(wire.Protocols)[0].Requirements, callerReqs) {
		t.Fatalf("wire webdav requirements = %#v", webdavs(wire.Protocols)[0].Requirements)
	}
	if !slicesEqual(webapps(wire.Protocols)[0].Requirements, callerReqs) {
		t.Fatalf("wire webapp requirements = %#v", webapps(wire.Protocols)[0].Requirements)
	}
	if !sameStringSet(storedWebDAV(t, stored).Requirements, storedWebapp(t, stored).Requirements) {
		t.Fatal("stored webdav and webapp requirements are not the same set")
	}
	storedWebDAV(t, stored).Requirements[0] = "mutated"
	if callerReqs[0] != reqMustExchangeToken || davCaller.GetWebdavOptions().Requirements[0] != reqMustExchangeToken {
		t.Fatal("stored webdav requirements alias the caller")
	}
}

func TestCreateOCMShareWebDAVOnlyRequirementsStay(t *testing.T) {
	reqs := []string{"legacy-req"}
	caller := webdavMethodWith(reqs)
	wire, stored, _, _ := shareFixture{
		offer:   true,
		methods: []*ocm.AccessMethod{caller},
		disco:   capableDiscovery(),
	}.run(t)
	if countKind(wire.Protocols, "webapp") != 0 {
		t.Fatal("webdav-only share gained a webapp protocol")
	}
	if !slicesEqual(storedWebDAV(t, stored).Requirements, reqs) {
		t.Fatalf("stored requirements = %#v", storedWebDAV(t, stored).Requirements)
	}
	if !slicesEqual(webdavs(wire.Protocols)[0].Requirements, reqs) {
		t.Fatalf("wire requirements = %#v", webdavs(wire.Protocols)[0].Requirements)
	}
	storedWebDAV(t, stored).Requirements[0] = "mutated"
	if reqs[0] != "legacy-req" {
		t.Fatal("stored webdav requirements alias the caller")
	}
}

func TestCreateOCMSharePaddedAppNameAndAlias(t *testing.T) {
	shared := webappMethod("incoming-name")
	before := proto.Clone(shared).(*ocm.AccessMethod)
	methods := []*ocm.AccessMethod{webdavMethod(), shared}
	wire, stored, _, _ := shareFixture{
		offer:   true,
		name:    testPaddedName,
		methods: methods,
		disco:   capableDiscovery(),
	}.run(t)
	if !proto.Equal(before, shared) {
		t.Fatalf("caller method changed: before=%v after=%v", before, shared)
	}
	if shared.GetWebappOptions().AppName != "incoming-name" {
		t.Fatal("caller app name changed")
	}
	got := webapps(wire.Protocols)
	if len(got) != 1 || got[0].AppName != testPaddedName {
		t.Fatalf("wire appName = %#v, want %q", got, testPaddedName)
	}
	storedOpts := storedWebapp(t, stored)
	if storedOpts.AppName != testPaddedName {
		t.Fatalf("stored appName = %q", storedOpts.AppName)
	}
	if stored.AccessMethods[1] == shared || storedOpts == shared.GetWebappOptions() {
		t.Fatal("stored webapp aliases the request method")
	}
	if got[0].URI != testOpener || strings.Contains(got[0].URI, "/lab") {
		t.Fatalf("uri = %q", got[0].URI)
	}
	if !slicesEqual(got[0].Targets, []string{"blank"}) {
		t.Fatalf("targets = %#v", got[0].Targets)
	}
}

func sameSlice(a, b []string) bool {
	return len(a) > 0 && len(b) > 0 && &a[0] == &b[0]
}
