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

package http

import (
	"context"
	"errors"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

const (
	fileID  = "eoshome-i01$L2VvcwB!4711"
	shareID = "1234"
	linkID  = "5678"
)

// server answers like ocgraph, OCS and ocdav, with canned bodies, and keeps
// the requests.
type server struct {
	mu   sync.Mutex
	reqs []string
	ts   *httptest.Server
}

func (s *server) handle(w nethttp.ResponseWriter, r *nethttp.Request) {
	s.mu.Lock()
	b, _ := io.ReadAll(r.Body)
	s.reqs = append(s.reqs, r.Method+" "+r.URL.Path+" "+r.Header.Get("x-access-token")+" "+string(b))
	s.mu.Unlock()
	switch {
	case r.URL.Path == "/graph/v1.0/me":
		io.WriteString(w, `{"id":"uid-1","displayName":"Alice","mail":"alice@example.org","onPremisesSamAccountName":"alice"}`)
	case r.URL.Path == "/graph/v1beta1/me/drive/sharedByMe":
		io.WriteString(w, `{"value":[{"id":"`+fileID+`","name":"report.pdf","permissions":[
			{"id":"`+shareID+`","roles":["fb6c3e19-e378-47e5-b277-9732f9de6e21"],"grantedToV2":{"user":{"id":"bob","displayName":"Bob"}},"expirationDateTime":"2026-12-31T00:00:00Z",
			 "createdDateTime":"2026-10-01T10:00:00Z","invitation":{"invitedBy":{"user":{"id":"alice"}}}},
			{"id":"`+linkID+`","link":{"type":"view","webUrl":"https://box.example.org/s/AbCdEf","@libre.graph.permissions.link.hasPassword":true,"@libre.graph.displayName":"for the team"}}]}]}`)
	case r.URL.Path == "/graph/v1beta1/me/drive/sharedWithMe":
		io.WriteString(w, `{"value":[{"id":"a0ca6a90-a365-4782-871e-d44447bbc668$a0ca6a90-a365-4782-871e-d44447bbc668!99","name":"data",
			"@client.synchronize":true,"createdBy":{"user":{"id":"carol"}},
			"remoteItem":{"id":"eosproject$L2VvcwB!12","name":"data","permissions":[{"id":"99","roles":["b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"],"grantedToV2":{"group":{"id":"cern-users"}}}]}}]}`)
	case strings.HasPrefix(r.URL.Path, "/graph/v1beta1/drives/") && r.Method == nethttp.MethodPatch:
		io.WriteString(w, `{"id":"`+shareID+`","roles":["b1e2218d-eef8-4d4c-b82d-0f1a1b48f3b5"],"grantedToV2":{"user":{"id":"bob"}}}`)
	case r.URL.Path == "/ocs/v2.php/apps/files_sharing/api/v1/shares/"+shareID && r.Method == nethttp.MethodPut:
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":200},"data":{"id":"`+shareID+`"}}}`)
	case r.URL.Path == "/ocs/v2.php/apps/files_sharing/api/v1/shares/"+shareID && r.Method == nethttp.MethodGet:
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":200},"data":[{"id":"`+shareID+`","share_type":0,"uid_owner":"alice","uid_file_owner":"alice",
			"permissions":15,"stime":1767225600,"expiration":"2026-12-31 00:00:00","item_source":"`+fileID+`","share_with":"bob","file_target":"/report.pdf"}]}}`)
	case r.URL.Path == "/ocs/v2.php/apps/files_sharing/api/v1/shares/"+linkID:
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":200},"data":[{"id":"`+linkID+`","share_type":3,"uid_owner":"alice","permissions":1,"stime":1767225600,
			"token":"AbCdEf","name":"for the team","item_source":"`+fileID+`","share_with":"***redacted***"}]}}`)
	case r.URL.Path == "/ocs/v2.php/apps/files_sharing/api/v1/shares/404":
		w.WriteHeader(nethttp.StatusNotFound)
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":404,"message":"share not found"},"data":[]}}`)
	case r.URL.Path == "/ocs/v1.php/cloud/user/clients":
		io.WriteString(w, `{"ocs":{"meta":{"statuscode":100},"data":[{"id":"client-1","name":"laptop","created_at":"2026-01-01T00:00:00Z"}]}}`)
	case strings.HasPrefix(r.URL.Path, "/remote.php/dav/files/alice/"):
		w.WriteHeader(nethttp.StatusMultiStatus)
		io.WriteString(w, `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:response>
			<d:href>/remote.php/dav/files/alice/eos/user/a/alice/report.pdf</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>
			<d:resourcetype/><d:getcontentlength>5</d:getcontentlength><d:getetag>"e1"</d:getetag><oc:fileid>`+fileID+`</oc:fileid>
			<oc:permissions>RGDNVW</oc:permissions><oc:checksums><oc:checksum>ADLER32:062c0215</oc:checksum></oc:checksums>
			</d:prop></d:propstat></d:response></d:multistatus>`)
	default:
		nethttp.NotFound(w, r)
	}
}

func newServer(t *testing.T) (*server, *Backend) {
	t.Helper()
	s := &server{}
	s.ts = httptest.NewServer(nethttp.HandlerFunc(s.handle))
	t.Cleanup(s.ts.Close)
	b, err := New(s.ts.URL, Options{
		HTTPClient: s.ts.Client(),
		Credentials: credFunc(func(context.Context) (Credential, error) {
			return Credential{Header: "x-access-token", Value: "reva-jwt"}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, b
}

type credFunc func(context.Context) (Credential, error)

func (f credFunc) Credential(ctx context.Context) (Credential, error) { return f(ctx) }

func TestStat(t *testing.T) {
	s, b := newServer(t)
	info, err := b.Files().Stat(context.Background(), &provider.Reference{Path: "/eos/user/a/alice/report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Id.GetOpaqueId() != "4711" || info.Etag != "e1" || info.Size != 5 || info.Checksum.GetSum() != "062c0215" {
		t.Errorf("info = %v", info)
	}
	// Not over HTTPS: the owner.
	if info.Owner != nil {
		t.Errorf("owner = %v, want none: PROPFIND does not give it", info.Owner)
	}
	if p := info.PermissionSet; !p.Delete || !p.AddGrant || !p.InitiateFileUpload || p.CreateContainer {
		t.Errorf("permissions = %v, want those of oc:permissions RGDNVW", p)
	}
	if !strings.Contains(s.reqs[len(s.reqs)-1], " reva-jwt ") {
		t.Errorf("request %q, want the reva token in x-access-token", s.reqs[len(s.reqs)-1])
	}
	if _, err := b.Files().Stat(context.Background(), &provider.Reference{ResourceId: &provider.ResourceId{OpaqueId: "x"}}); !errors.Is(err, backend.ErrNotAvailable) {
		t.Errorf("stat by id: %v, want not available", err)
	}
}

// TestSharesFromGraph checks the fields that Graph cannot fill: they stay
// empty.
func TestSharesFromGraph(t *testing.T) {
	_, b := newServer(t)
	list, err := b.Shares().List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("shares = %v, want the share without the link", list)
	}
	s := list[0]
	if s.Id.OpaqueId != shareID || s.ResourceId.GetOpaqueId() != "4711" || s.Grantee.GetUserId().GetOpaqueId() != "bob" || s.Expiration == nil {
		t.Errorf("share = %v", s)
	}
	if permissions.RoleFromResourcePermissions(s.Permissions.Permissions).Name != permissions.RoleEditor {
		t.Errorf("permissions = %v, want those of the editor role", s.Permissions)
	}
	if s.Ctime.GetSeconds() == 0 || s.Creator.GetOpaqueId() != "alice" {
		t.Errorf("share = %v, want the ctime and the creator of createdDateTime and invitedBy", s)
	}
	for name, empty := range map[string]bool{
		"grantee idp":       s.Grantee.GetUserId().GetIdp() == "",
		"grantee user type": s.Grantee.GetUserId().GetType() == 0,
		"mtime":             s.Mtime == nil,
		"owner":             s.Owner == nil,
	} {
		if !empty {
			t.Errorf("%s is set, but Graph has no source for it: %v", name, s)
		}
	}
}

func TestShareGetFromOCS(t *testing.T) {
	_, b := newServer(t)
	s, err := b.Shares().Get(context.Background(), shareID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Creator.GetOpaqueId() != "alice" || s.Ctime.GetSeconds() != 1767225600 || s.ResourceId.GetOpaqueId() != "4711" || s.Expiration == nil {
		t.Errorf("share = %v", s)
	}
	if s.Mtime != nil || s.Grantee.GetUserId().GetIdp() != "" {
		t.Errorf("share = %v: OCS has no mtime and no idp", s)
	}
	_, err = b.Shares().Get(context.Background(), "404")
	if se, ok := errors.AsType[*backend.StatusError](err); !ok || se.Code != rpc.Code_CODE_NOT_FOUND {
		t.Errorf("missing share: %v, want CODE_NOT_FOUND", err)
	}
	if _, err := b.Shares().Get(context.Background(), linkID); !backend.IsNotFound(err) {
		t.Errorf("a link as a share: %v, want not found", err)
	}
}

func TestShareUpdate(t *testing.T) {
	s, b := newServer(t)
	ref := &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: shareID}}}
	_, err := b.Shares().Update(context.Background(), &collaboration.UpdateShareRequest{Ref: ref,
		Field: &collaboration.UpdateShareRequest_UpdateField{Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
			Permissions: &collaboration.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(s.reqs, func(r string) bool {
		return strings.HasPrefix(r, "PUT /ocs/v2.php/apps/files_sharing/api/v1/shares/"+shareID) && strings.HasSuffix(r, " permissions=1")
	}) {
		t.Errorf("requests %q, want an OCS PUT of the viewer permissions", s.reqs)
	}

	// OCS has no expiration for a share: Graph has, on the item that OCS gives.
	_, err = b.Shares().Update(context.Background(), &collaboration.UpdateShareRequest{Ref: ref,
		Field: &collaboration.UpdateShareRequest_UpdateField{Field: &collaboration.UpdateShareRequest_UpdateField_Expiration{
			Expiration: timestamp(time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC))}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "PATCH /graph/v1beta1/drives/eoshome-i01$L2VvcwB/items/" + fileID + "/permissions/" + shareID
	if !slices.ContainsFunc(s.reqs, func(r string) bool { return strings.HasPrefix(r, want) && strings.Contains(r, "2027-01-31") }) {
		t.Errorf("requests %q, want %s with the expiration", s.reqs, want)
	}
}

func TestReceivedShares(t *testing.T) {
	s, b := newServer(t)
	list, err := b.Shares().Received(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("received = %v", list)
	}
	rs := list[0]
	if rs.Share.Id.OpaqueId != "99" || rs.State != collaboration.ShareState_SHARE_STATE_ACCEPTED ||
		rs.Share.Grantee.GetGroupId().GetOpaqueId() != "cern-users" || rs.Share.Creator.GetOpaqueId() != "carol" {
		t.Errorf("received share = %v", rs)
	}
	if rs.MountPoint != nil || rs.Share.Ctime != nil {
		t.Errorf("received share = %v: Graph has no mount point and no ctime", rs)
	}
	if err := b.Shares().HideReceived(context.Background(), "99"); err == nil {
		last := s.reqs[len(s.reqs)-1]
		if !strings.Contains(last, "/items/"+jailID("99")+" ") {
			t.Errorf("request %q, want the share jail id", last)
		}
	}
}

func TestLinks(t *testing.T) {
	_, b := newServer(t)
	list, err := b.Links().List(context.Background(), nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("links = %v, %v", list, err)
	}
	l := list[0]
	if l.Token != "AbCdEf" || !l.PasswordProtected || l.DisplayName != "for the team" || l.Id.OpaqueId != linkID {
		t.Errorf("link = %v", l)
	}
	if l.Ctime != nil || l.Creator != nil || l.Owner != nil {
		t.Errorf("link = %v: Graph has no ctime, creator or owner", l)
	}
	got, err := b.Links().Get(context.Background(), &link.PublicShareReference{Spec: &link.PublicShareReference_Token{Token: "AbCdEf"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Id.OpaqueId != linkID || got.Creator.GetOpaqueId() != "alice" || got.Ctime == nil {
		t.Errorf("link by token = %v, want the OCS link with its creator and ctime", got)
	}
}

func TestTokensHaveTheClientID(t *testing.T) {
	_, b := newServer(t)
	list, err := b.Tokens().List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("tokens = %v, %v", list, err)
	}
	if list[0].Password != "client-1" || list[0].Label != "laptop" || list[0].TokenScope != nil {
		t.Errorf("token = %v, want the client id as the id, and no scope", list[0])
	}
	if _, err := b.Tokens().Create(context.Background(), nil); !errors.Is(err, backend.ErrNotAvailable) {
		t.Errorf("token create: %v, want not available", err)
	}
}

func TestMe(t *testing.T) {
	_, b := newServer(t)
	u, err := b.Users().Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice" || u.Id.OpaqueId != "uid-1" || u.Id.Idp != "" {
		t.Errorf("user = %v, want alice without an idp", u)
	}
}
