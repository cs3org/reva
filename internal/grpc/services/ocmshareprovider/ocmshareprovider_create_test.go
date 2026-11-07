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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
)

func TestCreateOCMShareWireAndStoreMatch(t *testing.T) {
	wire, stored, _, _ := shareFixture{
		offer: false,
		methods: []*ocm.AccessMethod{
			webdavMethod(),
			webappMethod(),
		},
		disco: capableDiscovery(),
	}.run(t)
	assertNoWebapp(t, wire, stored)
	assertWebDAVRetained(t, wire, stored)
	if len(stored.AccessMethods) != 1 {
		t.Fatalf("stored methods = %d, want the webdav-only offer", len(stored.AccessMethods))
	}
	if countKind(wire.Protocols, "webdav") != 1 || countKind(wire.Protocols, "webapp") != 0 {
		t.Fatalf("wire protocols = %#v", wire.Protocols)
	}
}

func TestCreateOCMShareOpenerIgnoresOpaqueID(t *testing.T) {
	ids := []string{"opaque-aaa", "opaque-bbb"}
	var uris []string
	for _, id := range ids {
		wire, _, _, _ := shareFixture{
			offer:   true,
			shareID: id,
			methods: []*ocm.AccessMethod{webdavMethod(), webappMethod()},
			disco:   capableDiscovery(),
		}.run(t)
		got := webapps(wire.Protocols)
		if len(got) != 1 {
			t.Fatalf("id %s webapps = %d", id, len(got))
		}
		if got[0].URI != testOpener {
			t.Fatalf("id %s uri = %q", id, got[0].URI)
		}
		if strings.Contains(got[0].URI, id) || strings.Contains(got[0].URI, "/lab") {
			t.Fatalf("uri %q contains share id or /lab", got[0].URI)
		}
		if !slicesEqual(got[0].Targets, []string{"blank"}) {
			t.Fatalf("id %s targets = %#v", id, got[0].Targets)
		}
		uris = append(uris, got[0].URI)
	}
	if uris[0] != uris[1] {
		t.Fatalf("openers differ: %q vs %q", uris[0], uris[1])
	}
}

func TestCreateOCMShareRemoteFailureDoesNotStore(t *testing.T) {
	t.Run("rejection", func(t *testing.T) {
		shareFixture{
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod()},
			disco:      capableDiscovery(),
			postStatus: http.StatusBadRequest,
		}.run(t)
	})
	t.Run("transport", func(t *testing.T) {
		shareFixture{
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod()},
			disco:    capableDiscovery(),
			dropPost: true,
		}.run(t)
	})
}

func TestCreateOCMShareConfiguredNameEscapesOnWire(t *testing.T) {
	wire, stored, _, _ := shareFixture{
		offer:   true,
		name:    testQuotedName,
		methods: []*ocm.AccessMethod{webdavMethod(), webappMethod()},
		disco:   capableDiscovery(),
	}.run(t)
	got := webapps(wire.Protocols)
	if len(got) != 1 || len(got[0].Apps) != 1 || got[0].Apps[0].Name != testQuotedName {
		t.Fatalf("wire apps = %#v, want %q", got, testQuotedName)
	}
	if storedWebapp(t, stored) == nil {
		t.Fatal("stored share lost the webapp access method")
	}
}

func TestCreateOCMShareRepeatedCreates(t *testing.T) {
	methods := []*ocm.AccessMethod{webdavMethod(), webappMethod()}
	firstWire, firstStored, _, _ := shareFixture{
		offer:   true,
		methods: methods,
		disco:   capableDiscovery(),
	}.run(t)
	firstStored.AccessMethods[1].GetWebappOptions().Requirements[0] = "mutated-stored"
	if share.DefaultWebappRequirements[0] != reqMustExchangeToken {
		t.Fatal("mutating a stored share changed the package default")
	}

	secondWire, secondStored, repo, _ := shareFixture{
		offer:   true,
		methods: methods,
		disco:   capableDiscovery(),
	}.run(t)
	if repo.storeN != 1 {
		t.Fatalf("second create stores = %d, want 1", repo.storeN)
	}
	if !slicesEqual(webapps(secondWire.Protocols)[0].Requirements, share.DefaultWebappRequirements) {
		t.Fatalf("second wire requirements = %#v", webapps(secondWire.Protocols)[0].Requirements)
	}
	if !slicesEqual(storedWebapp(t, secondStored).Requirements, share.DefaultWebappRequirements) {
		t.Fatalf("second stored requirements = %#v", storedWebapp(t, secondStored).Requirements)
	}
	if webapps(firstWire.Protocols)[0].URI != webapps(secondWire.Protocols)[0].URI {
		t.Fatal("repeated creates changed the opener")
	}
}

func TestWebappProtocolBuilderDoesNotDefaultRequirements(t *testing.T) {
	svc := &service{conf: &config{WebAppEndpoint: testOpener}}
	protocol := svc.getWebappProtocol(&ocm.Share{Token: secretMarker}, &ocm.AccessMethod_WebappOptions{
		WebappOptions: &ocm.WebappAccessMethod{
			Permissions: viewerPerm(),
		},
	}, nil)
	raw, err := json.Marshal(protocol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "apps") {
		t.Fatalf("empty apps were serialized: %s", raw)
	}
	if protocol.URI != testOpener {
		t.Fatalf("uri = %q", protocol.URI)
	}
	if strings.Contains(protocol.URI, "/lab") {
		t.Fatalf("uri %q is not the bare endpoint", protocol.URI)
	}
	if !slicesEqual(protocol.Targets, []string{"blank"}) {
		t.Fatalf("targets = %#v", protocol.Targets)
	}
	if len(protocol.Requirements) != 0 {
		t.Fatalf("wire builder defaulted requirements: %#v", protocol.Requirements)
	}
	protocol.Targets[0] = "mutated"
	protocol.Requirements = append(protocol.Requirements, "mutated")
	if share.DefaultWebappTargets[0] != "blank" {
		t.Fatal("wire target mutation changed the package default")
	}
}

// A resource without a supported type is refused before an ID is generated,
// the peer is contacted or anything is stored.
func TestCreateOCMShareRefusesUnsupportedResourceType(t *testing.T) {
	for _, rt := range []providerpb.ResourceType{providerpb.ResourceType_RESOURCE_TYPE_REFERENCE, providerpb.ResourceType_RESOURCE_TYPE_SYMLINK, providerpb.ResourceType_RESOURCE_TYPE_INVALID} {
		t.Run(rt.String(), func(t *testing.T) {
			peer := &remotePeer{disco: capableDiscovery().payload()}
			srv := httptest.NewServer(http.HandlerFunc(peer.handler))
			t.Cleanup(srv.Close)
			repo := &capturingRepo{}
			svc := &service{conf: &config{ProviderDomain: "sender.example", WebDAVEndpoint: testWebDAVRoot}, repo: repo, client: ocmd.NewClient(2*time.Second, true)}
			stampGateway(&statGateway{info: &providerpb.ResourceInfo{Path: "/files/x", Type: rt, Owner: &userpb.UserId{OpaqueId: "einstein", Idp: "sender.example"}}})
			ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "sender.example"}})
			resp, err := svc.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
				ResourceId: &providerpb.ResourceId{StorageId: "storage", OpaqueId: "resource"},
				Grantee: &providerpb.Grantee{Type: providerpb.GranteeType_GRANTEE_TYPE_USER, Id: &providerpb.Grantee_UserId{
					UserId: &userpb.UserId{OpaqueId: "marie", Idp: "remote.example"},
				}},
				RecipientMeshProvider: &ocmprovider.ProviderInfo{Domain: "remote.example", Services: []*ocmprovider.Service{{
					Endpoint: &ocmprovider.ServiceEndpoint{Type: &ocmprovider.ServiceType{Name: "OCM"}, Path: srv.URL + "/ocm"},
				}}},
				AccessMethods: []*ocm.AccessMethod{webdavMethod()},
			})
			if err != nil || resp.GetStatus().GetCode() != rpc.Code_CODE_INVALID_ARGUMENT {
				t.Fatalf("%v %v", resp, err)
			}
			if posts, _ := peer.posted(); posts != 0 || repo.storeN != 0 || repo.idN != 0 {
				t.Fatalf("posts %d stores %d ids %d", posts, repo.storeN, repo.idN)
			}
		})
	}
}
