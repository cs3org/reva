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
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/embedded"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestNewWiresCreateOCMShare proves New() builds the OCM client and that
// CreateOCMShare uses it. The service is not assembled by hand and the
// client is not injected.
func TestNewWiresCreateOCMShare(t *testing.T) {
	const driver = "new-wiring-test"
	repo := &capturingRepo{nextID: "opaque-share"}
	withTestDrivers(
		t,
		driver,
		func(context.Context, map[string]any) (share.Repository, error) {
			return repo, nil
		},
		func(context.Context, map[string]any) (embedded.Transferrer, error) {
			return noopTransferrer{}, nil
		},
	)

	peer := &remotePeer{disco: capableDiscovery().payload()}
	srv := httptest.NewTLSServer(http.HandlerFunc(peer.handler))
	t.Cleanup(srv.Close)

	raw, err := New(context.Background(), map[string]any{
		"driver":          driver,
		"embedded_driver": driver,
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"offer_webapp":    true,
		"webapp_name":     testAppName,
		"webapp_endpoint": testOpener,
		"client_insecure": true,
		"client_timeout":  2,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc, ok := raw.(*service)
	if !ok || svc == nil || svc.client == nil {
		t.Fatal("New did not return a service wired with an OCM client")
	}
	if !svc.conf.OfferWebapp || svc.conf.WebappName != testAppName || svc.conf.WebAppEndpoint != testOpener {
		t.Fatalf("wired offer config = %#v", svc.conf)
	}
	if !svc.conf.ClientInsecure || svc.conf.ClientTimeout != 2 {
		t.Fatalf("client config insecure=%t timeout=%d", svc.conf.ClientInsecure, svc.conf.ClientTimeout)
	}

	stampGateway(&statGateway{info: &providerpb.ResourceInfo{
		Path: "/files/notes.txt",
		Type: providerpb.ResourceType_RESOURCE_TYPE_FILE,
		Owner: &userpb.UserId{
			OpaqueId: "einstein",
			Idp:      "sender.example",
		},
	}})
	user := &userpb.User{
		Id:          &userpb.UserId{OpaqueId: "einstein", Idp: "sender.example"},
		DisplayName: "Albert",
	}
	logger := zerolog.Nop()
	ctx := appctx.ContextSetUser(context.Background(), user)
	ctx = appctx.WithLogger(ctx, &logger)
	resp, err := svc.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
		ResourceId: &providerpb.ResourceId{StorageId: "storage", OpaqueId: "resource"},
		Grantee: &providerpb.Grantee{
			Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerpb.Grantee_UserId{
				UserId: &userpb.UserId{OpaqueId: "marie", Idp: "remote.example"},
			},
		},
		RecipientMeshProvider: &ocmprovider.ProviderInfo{
			Domain: "remote.example",
			Services: []*ocmprovider.Service{
				{
					Endpoint: &ocmprovider.ServiceEndpoint{
						Type: &ocmprovider.ServiceType{Name: "OCM"},
						Path: srv.URL + "/ocm",
					},
				},
			},
		},
		AccessMethods: []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
	})
	if got := status.Code(err); got != codes.OK {
		t.Fatalf("grpc status = %s, want %s", got, codes.OK)
	}
	if statusOf(resp) == nil || statusOf(resp).Code != rpc.Code_CODE_OK {
		t.Fatalf("status = %#v, want CODE_OK", statusOf(resp))
	}
	posts, body := peer.posted()
	if posts != 1 {
		t.Fatalf("remote posts = %d, want 1 body=%s", posts, body)
	}
	var wire ocmd.NewShareRequest
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal wire share: %v body=%s", err, body)
	}
	if countKind(wire.Protocols, "webapp") != 1 || countKind(wire.Protocols, "webdav") != 1 {
		t.Fatalf(
			"wire webapp = %d, webdav = %d, want 1 and 1",
			countKind(wire.Protocols, "webapp"),
			countKind(wire.Protocols, "webdav"),
		)
	}
	if repo.storeN != 1 || repo.stored == nil {
		t.Fatalf("store count = %d, stored set = %t, want 1 and set", repo.storeN, repo.stored != nil)
	}
	if countWebappStored(repo.stored.AccessMethods) != 1 {
		t.Fatalf("stored webapp count = %d, want 1", countWebappStored(repo.stored.AccessMethods))
	}
}
