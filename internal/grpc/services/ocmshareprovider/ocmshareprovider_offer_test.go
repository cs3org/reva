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
	"strings"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/protobuf/proto"
)

func TestCreateOCMShareOfferGate(t *testing.T) {
	graphMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("")}
	ocsMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("")}
	cliMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("not-configured")}

	cases := []struct {
		name       string
		offer      bool
		methods    []*ocm.AccessMethod
		disco      discoverySpec
		resource   providerpb.ResourceType
		wantWebapp int
		wantWebdav int
		appName    string
		endpoint   string
		marker     string
	}{
		{
			name:       "gate absent remote capable",
			offer:      false,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod(secretMarker)},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
			marker:     secretMarker,
		},
		{
			name:       "gate false remote capable",
			offer:      false,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("Other")},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "gate true both capabilities",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "gate true no webapp candidate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod()},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true missing webapp-receive",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true targets omit blank",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"iframe"},
				folderTargets: []string{"iframe"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true missing exchange-token",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"blank"},
				folderTargets: []string{"blank"},
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "graph shaped empty name",
			offer:      true,
			methods:    graphMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "ocs shaped empty name",
			offer:      true,
			methods:    ocsMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "cli shaped different name",
			offer:      true,
			methods:    cliMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:  "every webapp candidate removed",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod("one"),
				webappMethod("two"),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"blank"},
				folderTargets: []string{"blank"},
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "file uses same predicate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource:   providerpb.ResourceType_RESOURCE_TYPE_FILE,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "folder uses same predicate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource:   providerpb.ResourceType_RESOURCE_TYPE_CONTAINER,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:     "file ignored when only folder can receive",
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource: providerpb.ResourceType_RESOURCE_TYPE_FILE,
			disco: discoverySpec{
				folderReceive: true,
				folderTargets: []string{"blank"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:     "folder ignored when only file can receive",
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource: providerpb.ResourceType_RESOURCE_TYPE_CONTAINER,
			disco: discoverySpec{
				fileReceive:   true,
				fileTargets:   []string{"blank"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			before := proto.Clone(&ocm.CreateOCMShareRequest{AccessMethods: tt.methods}).(*ocm.CreateOCMShareRequest)
			logs := &bytes.Buffer{}
			wire, stored, _, _ := shareFixture{
				offer:    tt.offer,
				methods:  tt.methods,
				disco:    tt.disco,
				resource: tt.resource,
				marker:   tt.marker,
				logs:     logs,
			}.run(t)
			if countKind(wire.Protocols, "webapp") != tt.wantWebapp {
				t.Fatalf("wire webapp = %d, want %d", countKind(wire.Protocols, "webapp"), tt.wantWebapp)
			}
			if countWebappStored(stored.AccessMethods) != tt.wantWebapp {
				t.Fatalf("stored webapp = %d, want %d", countWebappStored(stored.AccessMethods), tt.wantWebapp)
			}
			if countKind(wire.Protocols, "webdav") != tt.wantWebdav {
				t.Fatalf("wire webdav = %d, want %d", countKind(wire.Protocols, "webdav"), tt.wantWebdav)
			}
			if !proto.Equal(before, &ocm.CreateOCMShareRequest{AccessMethods: tt.methods}) {
				t.Fatalf("request methods changed: before=%v after=%v", before.AccessMethods, tt.methods)
			}
			if tt.wantWebdav == 1 {
				assertWebDAVRetained(t, wire, stored)
			}
			omission := strings.Count(logs.String(), webappOmissionReason)
			requestedWebapp := countWebappStored(tt.methods)
			if tt.wantWebapp == 0 && requestedWebapp > 0 {
				if omission != 1 {
					t.Fatalf("omission logs = %d, want 1; logs=%s", omission, logs.String())
				}
			} else if omission != 0 {
				t.Fatalf("unexpected omission log: %s", logs.String())
			}
			if tt.wantWebapp == 0 {
				assertNoWebapp(t, wire, stored)
				return
			}
			got := webapps(wire.Protocols)
			if len(got) != 1 {
				t.Fatalf("wire webapps = %d", len(got))
			}
			if got[0].AppName != tt.appName {
				t.Fatalf("wire appName = %q, want %q", got[0].AppName, tt.appName)
			}
			if got[0].URI != tt.endpoint {
				t.Fatalf("wire uri = %q, want %q", got[0].URI, tt.endpoint)
			}
			if !slicesEqual(got[0].Permissions, []string{"read"}) {
				t.Fatalf("wire permissions = %#v, want %#v", got[0].Permissions, []string{"read"})
			}
			if !slicesEqual(got[0].Targets, share.DefaultWebappTargets) {
				t.Fatalf("wire targets = %#v", got[0].Targets)
			}
			if !slicesEqual(got[0].Requirements, share.DefaultWebappRequirements) {
				t.Fatalf("wire requirements = %#v", got[0].Requirements)
			}
			storedOpts := storedWebapp(t, stored)
			if storedOpts.AppName != tt.appName {
				t.Fatalf("stored appName = %q, want %q", storedOpts.AppName, tt.appName)
			}
			if !slicesEqual(storedOpts.Requirements, share.DefaultWebappRequirements) {
				t.Fatalf("stored requirements = %#v", storedOpts.Requirements)
			}
			if storedOpts.ProtoReflect().Descriptor().Fields().ByName("targets") != nil {
				t.Fatal("stored webapp access method invented a targets field")
			}
		})
	}
}
