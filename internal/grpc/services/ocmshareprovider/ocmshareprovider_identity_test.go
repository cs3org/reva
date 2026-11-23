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
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	ocmclient "github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	_ "github.com/cs3org/reva/v3/pkg/ocm/share/repository/json"
)

// createWith runs CreateOCMShare from local user opaque ID to the grantee and
// returns the peer it posted to.
func createWith(t *testing.T, providerDomain, userID string, grantee *userpb.UserId) (*remotePeer, *capturingRepo, *ocm.CreateOCMShareResponse) {
	t.Helper()
	peer := &remotePeer{disco: capableDiscovery().payload()}
	srv := httptest.NewServer(http.HandlerFunc(peer.handler))
	t.Cleanup(srv.Close)
	repo := &capturingRepo{}
	key, err := httpsig.LoadKey(writeKeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	svc := &service{conf: &config{ProviderDomain: providerDomain, WebDAVEndpoint: testWebDAVRoot}, repo: repo, client: ocmd.NewClient(2*time.Second, true), signingKey: key}
	stampGateway(&statGateway{info: &providerpb.ResourceInfo{Path: "/files/x", Type: providerpb.ResourceType_RESOURCE_TYPE_FILE,
		Owner: &userpb.UserId{OpaqueId: userID, Idp: "ignored.example.org"}}})
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: userID, Idp: "ignored.example.org"}})
	resp, err := svc.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
		ResourceId: &providerpb.ResourceId{StorageId: "storage", OpaqueId: "resource"},
		Grantee:    &providerpb.Grantee{Type: providerpb.GranteeType_GRANTEE_TYPE_USER, Id: &providerpb.Grantee_UserId{UserId: grantee}},
		RecipientMeshProvider: &ocmprovider.ProviderInfo{Domain: "remote.example", Services: []*ocmprovider.Service{{
			Endpoint: &ocmprovider.ServiceEndpoint{Type: &ocmprovider.ServiceType{Name: "OCM"}, Path: srv.URL + "/ocm"},
		}}},
		AccessMethods: []*ocm.AccessMethod{webdavMethod()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return peer, repo, resp
}

// The outgoing addresses keep every opaque ID byte and the complete hosts,
// so the receiver recovers both by splitting at the last @.
func TestCreateOCMShareWireKeepsExactAddresses(t *testing.T) {
	for _, tc := range []struct {
		userID, granteeID, granteeIdp string
		shareWith, sender             string
	}{
		{"alice@a.example.org", "bob@b.example.org", "b.example.org", "bob@b.example.org@b.example.org", "alice@a.example.org@a.example.org:8443"},
		{"alice@a.example.org@a.example.org", "bob@b.example.org@b.example.org", "B.Example.org", "bob@b.example.org@b.example.org@b.example.org", "alice@a.example.org@a.example.org@a.example.org:8443"},
		{"alice@other", "bob@other", "b.example.org:8443", "bob@other@b.example.org:8443", "alice@other@a.example.org:8443"},
		{" id ", " id ", "[2001:db8::1]:8443", " id @[2001:db8::1]:8443", " id @a.example.org:8443"},
	} {
		peer, _, resp := createWith(t, "a.example.org:8443", tc.userID, &userpb.UserId{OpaqueId: tc.granteeID, Idp: tc.granteeIdp})
		posts, body := peer.posted()
		if resp.GetStatus().GetCode() != rpc.Code_CODE_OK || posts != 1 {
			t.Fatalf("%q: %v posts %d", tc.granteeID, resp.GetStatus(), posts)
		}
		var wire ocmd.NewShareRequest
		if err := json.Unmarshal(body, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.ShareWith != tc.shareWith || wire.Sender != tc.sender || wire.Owner != tc.sender {
			t.Fatalf("wire shareWith %q sender %q owner %q", wire.ShareWith, wire.Sender, wire.Owner)
		}
		if id, host, err := providerdomain.ParseOCMAddress(wire.ShareWith); err != nil || id != tc.granteeID || host != strings.ToLower(tc.granteeIdp) {
			t.Fatalf("shareWith splits into %q %q %v", id, host, err)
		}
		if id, host, err := providerdomain.ParseOCMAddress(wire.Sender); err != nil || id != tc.userID || host != "a.example.org:8443" {
			t.Fatalf("sender splits into %q %q %v", id, host, err)
		}
	}
}

// A grantee without a valid separate host fails before any peer call.
func TestCreateOCMShareRefusesInvalidGrantee(t *testing.T) {
	for _, g := range []*userpb.UserId{
		{OpaqueId: "bob", Idp: "https://b.example.org"},
		{OpaqueId: "bob", Idp: "x@b.example.org"},
		{OpaqueId: "bob", Idp: "b.example.org/path"},
		{OpaqueId: "bob", Idp: "b.example.org:bad"},
		{OpaqueId: "bob", Idp: ""},
		{OpaqueId: "", Idp: "b.example.org"},
	} {
		peer, repo, resp := createWith(t, "a.example.org", "alice", g)
		posts, _ := peer.posted()
		if resp.GetStatus().GetCode() != rpc.Code_CODE_INVALID_ARGUMENT || posts != 0 || peer.discoveries() != 0 || repo.idN != 0 || repo.storeN != 0 {
			t.Fatalf("%v: %v posts %d discoveries %d ids %d", g, resp.GetStatus(), posts, peer.discoveries(), repo.idN)
		}
	}
}

// provider_domain is checked and canonicalized before anything is allocated.
func TestNewChecksProviderDomain(t *testing.T) {
	keyFile := writeKeyFile(t)
	drivers := map[string]any{"json": map[string]any{"file": filepath.Join(t.TempDir(), "missing", "ocm.json")}}
	for _, bad := range []string{"https://b.example.org", "b.example.org/x", "x@b.example.org"} {
		if _, err := New(context.Background(), map[string]any{"provider_domain": bad, "driver": "json", "drivers": drivers, "gatewaysvc": "localhost:0", "webdav_endpoint": testWebDAVRoot, "signing_key": keyFile}); err == nil ||
			err.Error() != "ocmshareprovider: provider_domain is not an OCM host" {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	good := map[string]any{"json": map[string]any{"file": filepath.Join(t.TempDir(), "ocm.json")}}
	s, err := New(context.Background(), map[string]any{"provider_domain": "B.Example.ORG:8443", "driver": "json", "drivers": good, "embedded_driver": testDriverName, "gatewaysvc": "localhost:0", "webdav_endpoint": testWebDAVRoot, "signing_key": keyFile})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	svc := s.(*service)
	if got := svc.conf.ProviderDomain; got != "b.example.org:8443" {
		t.Fatalf("provider domain %q", got)
	}
	notifier := svc.notifier.(*ocmd.AsyncNotifier)
	if notifier.Key == nil || notifier.Key != svc.signingKey {
		t.Fatal("notifier did not receive the service signing key")
	}
	if _, err := notifier.Client.Discover(context.Background(), "https://127.0.0.1:1"); !errors.Is(err, ocmclient.ErrPolicyViolation) {
		t.Fatalf("notification discovery did not enforce the public-only policy: %v", err)
	}
}

type countingNotifier struct{ domains []string }

func (n *countingNotifier) NewNotification(_ context.Context, domain string, _ *ocmd.OutgoingNotification) ocmd.SendResult {
	n.domains = append(n.domains, domain)
	return ocmd.SendResult{}
}

// A stored peer host is validated as it is; an invalid one reaches no notifier.
func TestEmitValidatesThePeerHost(t *testing.T) {
	notice := ocmd.ShareStateNotification{Type: ocmd.NotificationShareUnshared, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1"}
	for _, bad := range []string{"https://b.example.org", "alice@b.example.org", "b.example.org:bad", ""} {
		n := &countingNotifier{}
		(&service{conf: &config{ProviderDomain: "a.example.org"}, notifier: n}).emit(context.Background(), bad, notice)
		if len(n.domains) != 0 {
			t.Fatalf("%q reached the notifier as %v", bad, n.domains)
		}
	}
	n := &countingNotifier{}
	(&service{conf: &config{ProviderDomain: "a.example.org"}, notifier: n}).emit(context.Background(), "B.example.org:8443", notice)
	if len(n.domains) != 1 || n.domains[0] != "b.example.org:8443" {
		t.Fatalf("valid target %v", n.domains)
	}
}

// The old formatter removed a sender suffix equal to the configured host.
func TestCreateOCMShareKeepsSenderSuffixOfThisHost(t *testing.T) {
	peer, _, resp := createWith(t, "a.example.org", "alice@a.example.org", &userpb.UserId{OpaqueId: "bob", Idp: "b.example.org"})
	_, body := peer.posted()
	var wire ocmd.NewShareRequest
	if err := json.Unmarshal(body, &wire); err != nil || resp.GetStatus().GetCode() != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", resp.GetStatus(), err)
	}
	if wire.Sender != "alice@a.example.org@a.example.org" || wire.Owner != "alice@a.example.org@a.example.org" {
		t.Fatalf("sender %q owner %q", wire.Sender, wire.Owner)
	}
}
