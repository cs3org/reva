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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmclient "github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"github.com/rs/zerolog"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const emitSecret = "received-secret-5b2d"

// notifyPeer is a remote OCM server recording received notifications.
type notifyPeer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	bodies  []map[string]any
	jwks    []byte
	signed  int
	status  int
	noCap   bool
	payload string
}

func newNotifyPeer(t *testing.T, jwks []byte) *notifyPeer {
	p := &notifyPeer{status: http.StatusCreated, jwks: jwks}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		switch r.URL.Path {
		case "/.well-known/ocm":
			caps := []string{"invites", "notifications"}
			if p.noCap {
				caps = []string{"invites"}
			}
			_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{Enabled: true, APIVersion: "1.4.0", Endpoint: p.srv.URL + "/ocm", Capabilities: caps})
		case "/ocm/notifications":
			raw, _ := io.ReadAll(r.Body)
			if err := httpsig.Verify(r, raw, p.srv.URL, func() ([]byte, error) { return p.jwks, nil }); err == nil {
				p.signed++
			}
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			p.bodies = append(p.bodies, body)
			w.WriteHeader(p.status)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *notifyPeer) host() string { return strings.TrimPrefix(p.srv.URL, "http://") }

func (p *notifyPeer) received() ([]map[string]any, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]any(nil), p.bodies...), p.signed
}

// syncNotifier sends synchronously so that tests observe every attempt.
type syncNotifier struct {
	c       *ocmd.OCMClient
	mu      sync.Mutex
	results []ocmd.SendResult

	key *httpsig.Key
}

func (n *syncNotifier) NewNotification(ctx context.Context, domain string, out *ocmd.OutgoingNotification) ocmd.SendResult {
	res := n.c.NewNotification(ctx, domain, out, n.key)
	n.mu.Lock()
	n.results = append(n.results, res)
	n.mu.Unlock()
	return res
}

// controlledTransferrer captures the completion callbacks of transfers.
type controlledTransferrer struct {
	mu        sync.Mutex
	callbacks []func(error)
	fail      error
}

func (c *controlledTransferrer) Process(_ context.Context, _ string, _ string, done func(error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.callbacks = append(c.callbacks, done)
	return nil
}

func (c *controlledTransferrer) callback(i int) func(error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callbacks[i]
}

// lockedBuffer is a log sink safe for concurrent RPCs.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type emitEnv struct {
	svc   *service
	repo  share.Repository
	peer  *notifyPeer
	xfer  *controlledTransferrer
	owner context.Context
	recip context.Context
	logs  *lockedBuffer
	// dbPath is the SQLite file of repo.
	dbPath string
}

func newEmitEnv(t *testing.T) *emitEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ocm.sqlite")
	repo, err := sqlmgr.NewOCMShareManager(context.Background(), map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	key, err := httpsig.LoadKey(writeKeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := key.JWKS()
	if err != nil {
		t.Fatal(err)
	}
	peer := newNotifyPeer(t, jwks)
	xfer := &controlledTransferrer{}
	env := &emitEnv{
		svc: &service{
			conf:        &config{ProviderDomain: "cernbox.example.org", WebDAVEndpoint: testWebDAVRoot},
			repo:        repo,
			client:      ocmd.NewClient(2*time.Second, false),
			transferrer: xfer,
			notifier: &syncNotifier{
				c:   ocmd.NewPublicOnlyClientWithConfig(ocmclient.TransportConfig{Timeout: 2 * time.Second, AllowLoopback: true}),
				key: key,
			},
		},
		repo:   repo,
		peer:   peer,
		xfer:   xfer,
		logs:   &lockedBuffer{},
		dbPath: dbPath,
	}
	logger := zerolog.New(env.logs)
	env.owner = appctx.WithLogger(appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox.example.org"},
	}), &logger)
	env.recip = appctx.WithLogger(appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "marie", Idp: "cernbox.example.org"},
	}), &logger)
	stampGateway(&statGateway{info: &providerpb.ResourceInfo{Path: "/files/doc.txt", Type: providerpb.ResourceType_RESOURCE_TYPE_FILE}})
	return env
}

func (e *emitEnv) outgoing(t *testing.T) *ocm.Share {
	t.Helper()
	return e.outgoingOfType(t, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE)
}

func (e *emitEnv) outgoingOfType(t *testing.T, resourceType ocm.SharedResourceType) *ocm.Share {
	t.Helper()
	id, err := e.repo.GenerateID(e.owner)
	if err != nil {
		t.Fatal(err)
	}
	now := &typespb.Timestamp{Seconds: uint64(time.Now().Unix())}
	owner := &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox.example.org"}
	sh, err := e.repo.StoreShare(e.owner, &ocm.Share{
		Id:         id,
		Token:      "outgoing-token-" + id.OpaqueId,
		Name:       "doc.txt",
		ResourceId: &providerpb.ResourceId{StorageId: "s", OpaqueId: "r-" + id.OpaqueId},
		Grantee: &providerpb.Grantee{Type: providerpb.GranteeType_GRANTEE_TYPE_USER, Id: &providerpb.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: "marie", Idp: e.peer.host()},
		}},
		Owner: owner, Creator: owner, Ctime: now, Mtime: now,
		RecipientType: ocm.RecipientType_RECIPIENT_TYPE_USER,
		AccessMethods: []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(permissions.NewViewerRole().CS3ResourcePermissions(), nil, nil),
			share.NewWebappAccessMethod(permissions.NewViewerRole().CS3ResourcePermissions(), share.DefaultWebappRequirements),
		},
	}, resourceType)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

func (e *emitEnv) received(t *testing.T, remoteID string, embedded bool, accessTypes ...ocm.AccessType) *ocm.ReceivedShare {
	t.Helper()
	now := &typespb.Timestamp{Seconds: uint64(time.Now().Unix())}
	protocols := []*ocm.Protocol{share.NewWebDAVProtocol("https://provider/dav", emitSecret,
		&ocm.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}, accessTypes, nil)}
	rt := ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE
	if embedded {
		protocols = append(protocols, share.NewEmbeddedProtocol(`{"@graph":[]}`))
		rt = ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED
	}
	origin := &userpb.UserId{OpaqueId: "alice", Idp: e.peer.host()}
	rs, err := e.repo.StoreReceivedShare(e.recip, &ocm.ReceivedShare{
		RemoteShareId: remoteID,
		Name:          "doc.txt",
		Grantee: &providerpb.Grantee{Type: providerpb.GranteeType_GRANTEE_TYPE_USER, Id: &providerpb.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: "marie"},
		}},
		// The owner fallback differs from the original sender.
		Owner:              &userpb.UserId{OpaqueId: "service-account", Idp: "elsewhere.example.org"},
		Creator:            origin,
		Ctime:              now,
		Mtime:              now,
		ShareType:          ocm.ShareType_SHARE_TYPE_USER,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		SharedResourceType: rt,
		State:              ocm.ShareState_SHARE_STATE_PENDING,
		Protocols:          protocols,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func (e *emitEnv) setState(t *testing.T, id *ocm.ShareId, state ocm.ShareState, destination string) *ocm.UpdateReceivedOCMShareResponse {
	t.Helper()
	res, err := e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
		Share:      &ocm.ReceivedShare{Id: id, State: state, Destination: destination},
		UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func requireNotices(t *testing.T, p *notifyPeer, want ...string) []map[string]any {
	t.Helper()
	got, signed := p.received()
	if len(got) != len(want) {
		t.Fatalf("notices %v, want types %v", got, want)
	}
	if signed != len(got) {
		t.Fatalf("verified %d of %d notices", signed, len(got))
	}
	for i, typ := range want {
		if got[i]["notificationType"] != typ {
			t.Fatalf("notice %d is %v, want %s", i, got[i]["notificationType"], typ)
		}
	}
	return got
}

func fileOf(n map[string]any) map[string]any {
	return n["notification"].(map[string]any)["file"].(map[string]any)
}

func TestRemoveOCMShareEmitsUnsharedOnce(t *testing.T) {
	e := newEmitEnv(t)
	sh := e.outgoing(t)
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}

	res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: ref})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	got := requireNotices(t, e.peer, ocmd.NotificationShareUnshared)[0]
	if got["senderDomain"] != "cernbox.example.org" || got["resourceType"] != "file" || fileOf(got)["providerId"] != sh.Id.OpaqueId {
		t.Fatalf("payload %v", got)
	}
	if raw, _ := json.Marshal(got); bytes.Contains(raw, []byte(sh.Token)) || bytes.Contains(raw, []byte("sharedSecret")) {
		t.Fatal("SHARE_UNSHARED exposed a share credential")
	}

	res, _ = e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: ref})
	if res.Status.Code == rpc.Code_CODE_OK {
		t.Fatal("repeat delete succeeded")
	}
	requireNotices(t, e.peer, ocmd.NotificationShareUnshared)
}

func TestRemoveOCMShareByKeyNotifiesAcknowledgedRemovals(t *testing.T) {
	for _, tc := range []struct {
		name string
		lost bool
	}{{"acknowledged", false}, {"lost commit acknowledgment", true}} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			first := e.outgoing(t)
			survivor := e.outgoing(t)
			duplicate := proto.CloneOf(first)
			id, err := e.repo.GenerateID(e.owner)
			if err != nil {
				t.Fatal(err)
			}
			duplicate.Id, duplicate.Token = id, "other-token-"+id.OpaqueId
			second, err := e.repo.StoreShare(e.owner, duplicate, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE)
			if err != nil {
				t.Fatal(err)
			}
			key := &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{
				Owner: first.Owner, ResourceId: first.ResourceId, Grantee: first.Grantee,
			}}}
			wantStatus := rpc.Code_CODE_OK
			if tc.lost {
				e.svc.repo = failingRepo{Repository: e.repo, loseCommitAcknowledgment: true}
				wantStatus = rpc.Code_CODE_INTERNAL
			}
			res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: key})
			if err != nil || res.GetStatus() == nil || res.GetStatus().GetCode() != wantStatus {
				t.Fatalf("removal response %v: %v", res.GetStatus(), err)
			}
			owner := appctx.ContextMustGetUser(e.owner)
			for _, removed := range []*ocm.Share{first, second} {
				ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: removed.Id}}
				if _, err := e.repo.GetShare(e.owner, owner, ref); !errors.Is(err, share.ErrShareNotFound) {
					t.Errorf("share %s remains available: %v", removed.Id.OpaqueId, err)
				}
			}
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: survivor.Id}}
			if got, err := e.repo.GetShare(e.owner, owner, ref); err != nil || got.GetToken() != survivor.Token {
				t.Errorf("unrelated share changed: %v", err)
			}
			if tc.lost {
				if attempts := len(e.svc.notifier.(*syncNotifier).results); attempts != 0 {
					t.Errorf("unknown commit made %d notification attempts", attempts)
				}
				requireNotices(t, e.peer)
				return
			}
			notices := requireNotices(t, e.peer, ocmd.NotificationShareUnshared, ocmd.NotificationShareUnshared)
			for i, removed := range []*ocm.Share{first, second} {
				if got := fileOf(notices[i])["providerId"]; got != removed.Id.OpaqueId || notices[i]["resourceType"] != "file" {
					t.Errorf("notice %d has provider ID %v and resource type %v", i, got, notices[i]["resourceType"])
				}
			}
		})
	}
}

type failingRepo struct {
	share.Repository
	loseCommitAcknowledgment bool
}

var errCommitAcknowledgmentLost = errors.New("commit acknowledgment lost")

func (r failingRepo) DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) ([]share.ShareRemoval, error) {
	if r.loseCommitAcknowledgment {
		receipt, err := r.Repository.DeleteShare(ctx, user, ref)
		if err != nil {
			return nil, err
		}
		return receipt, errCommitAcknowledgmentLost
	}
	return nil, errors.New("storage unavailable")
}

func (r failingRepo) UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, fields ...*ocm.UpdateOCMShareRequest_UpdateField) (*share.ShareUpdate, error) {
	if r.loseCommitAcknowledgment {
		receipt, err := r.Repository.UpdateShare(ctx, user, ref, fields...)
		if err != nil {
			return nil, err
		}
		return receipt, errCommitAcknowledgmentLost
	}
	return nil, errors.New("storage unavailable")
}

func (r failingRepo) UpdateReceivedShare(ctx context.Context, user *userpb.User, received *ocm.ReceivedShare, mask *field_mask.FieldMask) (*share.ReceivedShareUpdate, error) {
	if r.loseCommitAcknowledgment {
		receipt, err := r.Repository.UpdateReceivedShare(ctx, user, received, mask)
		if err != nil {
			return nil, err
		}
		return receipt, errCommitAcknowledgmentLost
	}
	return nil, errors.New("storage unavailable")
}

func TestStorageFailureEmitsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		lost bool
	}{{"refused mutation", false}, {"lost commit acknowledgment", true}} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			sh := e.outgoing(t)
			rs := e.received(t, "remote-f", false)
			e.svc.repo = failingRepo{Repository: e.repo, loseCommitAcknowledgment: tc.lost}
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}
			if res, _ := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{Ref: ref, Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
				Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil)},
			}}}); res.Status.Code == rpc.Code_CODE_OK {
				t.Fatal("failed update reported OK")
			}
			if res, _ := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: ref}); res.Status.Code == rpc.Code_CODE_OK {
				t.Fatal("failed delete reported OK")
			}
			if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.Status.Code == rpc.Code_CODE_OK {
				t.Fatal("failed received update reported OK")
			}
			if tc.lost {
				_, err := e.repo.GetShare(e.owner, appctx.ContextMustGetUser(e.owner), ref)
				if !errors.Is(err, share.ErrShareNotFound) {
					t.Fatalf("unacknowledged deletion did not commit: %v", err)
				}
				stored, err := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip),
					&ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
				if err != nil || stored.GetState() != ocm.ShareState_SHARE_STATE_ACCEPTED {
					t.Fatalf("unacknowledged acceptance did not commit: %v", err)
				}
			}
			if attempts := len(e.svc.notifier.(*syncNotifier).results); attempts != 0 {
				t.Fatalf("%d notification attempts", attempts)
			}
			requireNotices(t, e.peer)
		})
	}
}

func TestUpdateOCMShareEmitsOnlyPermissionChanges(t *testing.T) {
	e := newEmitEnv(t)
	sh := e.outgoing(t)
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}
	update := func(fields ...*ocm.UpdateOCMShareRequest_UpdateField) {
		t.Helper()
		res, err := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{Ref: ref, Field: fields})
		if err != nil || res.Status.Code != rpc.Code_CODE_OK {
			t.Fatalf("%v %v", res, err)
		}
	}
	webdav := func(p *providerpb.ResourcePermissions) *ocm.UpdateOCMShareRequest_UpdateField {
		return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: share.NewWebDavAccessMethod(p, nil, nil)}}
	}
	webapp := func(p *providerpb.ResourcePermissions, app string) *ocm.UpdateOCMShareRequest_UpdateField {
		method := share.NewWebappAccessMethod(p, nil)
		if app != "" {
			method.GetWebappOptions().AppNames = []string{app}
		}
		return &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: method}}
	}

	update()
	update(&ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{
		Expiration: &typespb.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())},
	}})
	update(webdav(permissions.NewViewerRole().CS3ResourcePermissions()))
	update(webapp(permissions.NewViewerRole().CS3ResourcePermissions(), "renamed"))
	requireNotices(t, e.peer)

	update(webdav(permissions.NewEditorRole().CS3ResourcePermissions()))
	got := requireNotices(t, e.peer, ocmd.NotificationShareChangePermission)[0]
	perms := fileOf(got)["permissions"].([]any)
	if len(perms) != 2 || perms[0] != "read" || perms[1] != "write" || fileOf(got)["providerId"] != sh.Id.OpaqueId {
		t.Fatalf("payload %v", got)
	}
	if _, has := got["notification"].(map[string]any)["sharedSecret"]; has {
		t.Fatal("SHARE_CHANGE_PERMISSION carries no secret")
	}
	update(webdav(permissions.NewEditorRole().CS3ResourcePermissions()))
	update(webapp(permissions.NewEditorRole().CS3ResourcePermissions(), ""))
	notices := requireNotices(t, e.peer, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission)
	if _, present := fileOf(notices[1])["permissions"]; present {
		t.Fatal("Webapp-only update must leave the WebDAV permissions snapshot absent")
	}
	update(webdav(&providerpb.ResourcePermissions{}))
	notices = requireNotices(t, e.peer, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission)
	if empty, ok := fileOf(notices[2])["permissions"].([]any); !ok || len(empty) != 0 {
		t.Fatalf("cleared WebDAV permissions must send an empty snapshot: %v", fileOf(notices[2]))
	}
}

func TestReceivedStateTransitionsEmitOnce(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-42", false)

	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "")
	got := requireNotices(t, e.peer, ocmd.NotificationShareAccepted)[0]
	if got["senderDomain"] != "cernbox.example.org" || fileOf(got)["providerId"] != "remote-42" || got["resourceType"] != "file" {
		t.Fatalf("payload %v", got)
	}
	if raw, _ := json.Marshal(got); bytes.Contains(raw, []byte(emitSecret)) || bytes.Contains(raw, []byte("sharedSecret")) {
		t.Fatal("SHARE_ACCEPTED exposed a share credential")
	}

	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "")
	if _, err := e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
		Share:      &ocm.ReceivedShare{Id: rs.Id, Hidden: true},
		UpdateMask: &field_mask.FieldMask{Paths: []string{"hidden"}},
	}); err != nil {
		t.Fatal(err)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)

	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_REJECTED, "")
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareDeclined)
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, "")
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_TRANSFERRING, "")
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareDeclined)
}

func TestConcurrentAcceptEmitsAtMostOnce(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-c", false)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
				Share:      &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			})
		}()
	}
	wg.Wait()
	cur, err := e.repo.GetReceivedShare(e.recip, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if err != nil {
		t.Fatal(err)
	}
	if cur.State == ocm.ShareState_SHARE_STATE_ACCEPTED {
		requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	} else {
		requireNotices(t, e.peer)
	}
}

func TestEmbeddedTransferEmitsOnlyOnCommittedCompletion(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-e", true)

	res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest")
	if res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("start: %v", res.Status)
	}
	requireNotices(t, e.peer) // TRANSFERRING is not acceptance
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Fatalf("concurrent start: %v", res.Status)
	}

	e.xfer.callback(0)(nil)
	got := requireNotices(t, e.peer, ocmd.NotificationShareAccepted)[0]
	if got["resourceType"] != "ro-crate" || fileOf(got)["providerId"] != "remote-e" {
		t.Fatalf("payload %v", got)
	}
	e.xfer.callback(0)(nil) // duplicate completion
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)

	// A failed copy is not the recipient declining.
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, "")
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest")
	e.xfer.callback(1)(errors.New("copy failed"))
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	cur, _ := e.repo.GetReceivedShare(e.recip, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if cur.State != ocm.ShareState_SHARE_STATE_REJECTED {
		t.Fatalf("state %v", cur.State)
	}

	// Reset, start a new transfer, then the old callback arrives.
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, "")
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest")
	e.xfer.callback(1)(nil)
	cur, _ = e.repo.GetReceivedShare(e.recip, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if cur.State != ocm.ShareState_SHARE_STATE_TRANSFERRING {
		t.Fatalf("old callback completed the new attempt: %v", cur.State)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	e.xfer.callback(2)(nil)
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted)

	// A transfer that never starts is rejected without a notice.
	other := e.received(t, "remote-g", true)
	e.xfer.fail = errors.New("cannot start")
	if res := e.setState(t, other.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code == rpc.Code_CODE_OK {
		t.Fatal("failed start reported OK")
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted)
}

// The ordinary accept route sends no destination. An embedded share only
// becomes ACCEPTED, and is only announced, when its copy completes.
func TestEmbeddedAcceptanceNeedsCompletedCopy(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-n", true)
	marie := &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}
	state := func() ocm.ShareState {
		t.Helper()
		cur, err := e.repo.GetReceivedShare(e.recip, marie, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
		if err != nil {
			t.Fatal(err)
		}
		return cur.State
	}

	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Fatalf("accept without destination: %v", res.Status)
	}
	if s := state(); s != ocm.ShareState_SHARE_STATE_PENDING || len(e.xfer.callbacks) != 0 {
		t.Fatalf("state %v, %d transfers", s, len(e.xfer.callbacks))
	}
	requireNotices(t, e.peer)

	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("accept: %v", res.Status)
	}
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Fatalf("accept without destination during the copy: %v", res.Status)
	}
	if s := state(); s != ocm.ShareState_SHARE_STATE_TRANSFERRING {
		t.Fatalf("state during the copy %v", s)
	}
	requireNotices(t, e.peer)
	e.xfer.callback(0)(nil)
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)

	// A repeated acceptance after success starts no second copy.
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Fatalf("repeated accept: %v", res.Status)
	}
	if s := state(); s != ocm.ShareState_SHARE_STATE_ACCEPTED || len(e.xfer.callbacks) != 1 {
		t.Fatalf("state %v, %d transfers", s, len(e.xfer.callbacks))
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)

	// Without a transferrer the share stays pending.
	other := e.received(t, "remote-o", true)
	e.svc.transferrer = nil
	if res := e.setState(t, other.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Fatalf("accept without transferrer: %v", res.Status)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
}

func TestSenderFailureKeepsCommittedChange(t *testing.T) {
	e := newEmitEnv(t)
	e.peer.status = http.StatusInternalServerError
	sh := e.outgoing(t)
	res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareUnshared)
	if _, err := e.repo.GetShare(e.owner, &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}); err == nil {
		t.Fatal("deletion was rolled back")
	}

	e.peer.noCap = true
	rs := e.received(t, "remote-n", false)
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v", res.Status)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareUnshared)
	if strings.Contains(e.logs.String(), emitSecret) || strings.Contains(e.logs.String(), sh.Token) {
		t.Fatal("secret logged")
	}
}

// unavailableGateway fails every Stat, as when the resource is already gone.
type unavailableGateway struct {
	gateway.GatewayAPIClient
	stats int
}

func (g *unavailableGateway) Stat(context.Context, *providerpb.StatRequest, ...grpc.CallOption) (*providerpb.StatResponse, error) {
	g.stats++
	return &providerpb.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func TestCommittedChangesAreNotifiedWithoutStat(t *testing.T) {
	e := newEmitEnv(t)
	folder := e.outgoingOfType(t, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER)
	file := e.outgoing(t)
	gw := &unavailableGateway{}
	stampGateway(gw)

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: folder.Id}}
	if res, err := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{Ref: ref, Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil)},
	}}}); err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	if res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: ref}); err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	key := &ocm.ShareReference{Spec: &ocm.ShareReference_Key{Key: &ocm.ShareKey{Owner: file.Owner, ResourceId: file.ResourceId, Grantee: file.Grantee}}}
	if res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: key}); err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	got := requireNotices(t, e.peer, ocmd.NotificationShareChangePermission, ocmd.NotificationShareUnshared, ocmd.NotificationShareUnshared)
	if got[0]["resourceType"] != "folder" || got[1]["resourceType"] != "folder" || fileOf(got[1])["providerId"] != folder.Id.OpaqueId ||
		got[2]["resourceType"] != "file" || fileOf(got[2])["providerId"] != file.Id.OpaqueId {
		t.Fatalf("payloads %v", got)
	}
	if gw.stats != 0 {
		t.Fatalf("emission used %d Stat calls", gw.stats)
	}
}

func TestOldGrantMutationWithUnavailableResourceType(t *testing.T) {
	for _, tc := range []struct {
		name, itemType string
		code           rpc.Code
	}{
		{"prior format", "", rpc.Code_CODE_OK},
		{"explicit unsupported type", "reference", rpc.Code_CODE_FAILED_PRECONDITION},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			sh := e.outgoing(t)
			gw := &unavailableGateway{}
			stampGateway(gw)
			db, err := gorm.Open(sqlite.Open(e.dbPath), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.OcmShare{}).Where("id = ?", sh.Id.OpaqueId).Update("item_type", tc.itemType).Error; err != nil {
				t.Fatal(err)
			}
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}
			owner := &userpb.User{Id: sh.Owner}
			before, err := e.repo.GetShare(e.owner, owner, ref)
			if err != nil {
				t.Fatal(err)
			}
			up, err := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{Ref: ref, Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
				Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil)},
			}}})
			if err != nil || up.GetStatus().GetCode() != tc.code {
				t.Errorf("update: %v %v", up, err)
			}
			if tc.code == rpc.Code_CODE_OK {
				got, err := e.repo.GetShare(e.owner, owner, ref)
				if err != nil || proto.Equal(got, before) || len(webdavPermissionNames(got)) != 2 {
					t.Errorf("permissions not persisted: %v %v", got, err)
				}
			}
			rm, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: ref})
			if err != nil || rm.GetStatus().GetCode() != tc.code {
				t.Errorf("remove: %v %v", rm, err)
			}
			got, err := e.repo.GetShare(e.owner, owner, ref)
			if tc.code == rpc.Code_CODE_OK {
				if got != nil || !errors.Is(err, share.ErrShareNotFound) {
					t.Errorf("withdrawn share remains available: %v %v", got, err)
				}
			} else if err != nil || !proto.Equal(got, before) {
				t.Errorf("corrupt share changed: %v %v", got, err)
			}
			if attempts := len(e.svc.notifier.(*syncNotifier).results); attempts != 0 || gw.stats != 0 {
				t.Errorf("notification attempts %d, Stat calls %d", attempts, gw.stats)
			}
			requireNotices(t, e.peer)
			if tc.code == rpc.Code_CODE_OK {
				logs := e.logs.String()
				for _, typ := range []string{ocmd.NotificationShareChangePermission, ocmd.NotificationShareUnshared} {
					if !strings.Contains(logs, `"type":"`+typ+`","message":"not sending OCM notification: share has no stored resource type"`) {
						t.Errorf("no skip reason for %s: %s", typ, logs)
					}
				}
			}
		})
	}
}

func TestUnmaskedAcceptanceStartsNoTransfer(t *testing.T) {
	e := newEmitEnv(t)
	rs := e.received(t, "remote-m", true)
	for _, paths := range [][]string{{"hidden"}, {}, nil} {
		res, err := e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
			Share:      &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED, Destination: "/home/dest", Hidden: true},
			UpdateMask: &field_mask.FieldMask{Paths: paths},
		})
		if err != nil || res.Status.Code != rpc.Code_CODE_OK {
			t.Fatalf("mask %v: %v %v", paths, res, err)
		}
	}
	cur, err := e.repo.GetReceivedShare(e.recip, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if err != nil || cur.State != ocm.ShareState_SHARE_STATE_PENDING || !cur.Hidden {
		t.Fatalf("share %v %v", cur, err)
	}
	if n := len(e.xfer.callbacks); n != 0 {
		t.Fatalf("%d transfers started", n)
	}
	requireNotices(t, e.peer)

	// The requested acceptance still starts one transfer.
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest"); res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("accept: %v", res.Status)
	}
	e.xfer.callback(0)(nil)
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
}

func TestServiceCloseStopsNotifications(t *testing.T) {
	e := newEmitEnv(t)
	e.svc.notifier = &ocmd.AsyncNotifier{Client: ocmd.NewClient(2*time.Second, false)}
	sh := e.outgoing(t)
	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.RemoveOCMShare(e.owner, &ocm.RemoveOCMShareRequest{Ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("%v %v", res, err)
	}
	requireNotices(t, e.peer)
	if _, err := e.repo.GetShare(e.owner, &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein"}}, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}); err == nil {
		t.Fatal("deletion was rolled back")
	}
}
