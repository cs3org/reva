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

package ocmshareprovider

import (
	"bytes"
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	"github.com/cs3org/reva/v3/pkg/ocm/embedded"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	jsonrepo "github.com/cs3org/reva/v3/pkg/ocm/share/repository/json"
	"github.com/cs3org/reva/v3/pkg/permissions"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const emitSecret = "received-secret-5b2d"

// notifyPeer is a remote OCM server recording received notifications.
type notifyPeer struct {
	srv           *httptest.Server
	mu            sync.Mutex
	bodies        []map[string]any
	jwks          []byte
	signed        int
	status        int
	noCap         bool
	payload       string
	discovers     int
	posted        chan struct{}
	holdPost      func(context.Context)
	holdDiscovery func(context.Context)
}

func newNotifyPeer(t *testing.T, jwks []byte) *notifyPeer {
	p := &notifyPeer{status: http.StatusCreated, jwks: jwks}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		var postStatus int
		var holdPost func(context.Context)
		defer func() {
			p.mu.Unlock()
			if holdPost != nil {
				holdPost(r.Context())
			}
			if postStatus != 0 {
				w.WriteHeader(postStatus)
			}
		}()
		switch r.URL.Path {
		case "/.well-known/ocm":
			p.discovers++
			if holdDiscovery := p.holdDiscovery; holdDiscovery != nil {
				p.mu.Unlock()
				holdDiscovery(r.Context())
				p.mu.Lock()
			}
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
			postStatus, holdPost = p.status, p.holdPost
			if p.posted != nil {
				close(p.posted)
				p.posted = nil
			}
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
	if out.StateNotice != nil {
		defer out.StateNotice.Release()
		if !out.StateNotice.StillCurrent() {
			return ocmd.SendResult{}
		}
	}
	if out.StillCurrent != nil && !out.StillCurrent(ctx) {
		return ocmd.SendResult{}
	}
	res := n.c.NewNotification(ctx, domain, out, n.key)
	n.mu.Lock()
	n.results = append(n.results, res)
	n.mu.Unlock()
	if out.StateNotice != nil {
		return ocmd.SendResult{}
	}
	return res
}

// controlledTransferrer captures the completion callbacks of transfers.
type controlledTransferrer struct {
	mu        sync.Mutex
	callbacks []func(error)
	fail      error
	park      func(func(error))
}

func (c *controlledTransferrer) Process(_ context.Context, _ string, _ string, done func(error)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.callbacks = append(c.callbacks, done)
	if c.park != nil {
		go c.park(done)
		c.park = nil
	}
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
	e := newEmitEnvWithDatabase(t, map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbPath,
	})
	e.dbPath = dbPath
	return e
}

func newEmitEnvWithDatabase(t *testing.T, dbConfig map[string]any) *emitEnv {
	t.Helper()
	repo, err := sqlmgr.NewOCMShareManager(context.Background(), dbConfig)
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
			conf:        &config{Driver: "sql", ProviderDomain: "cernbox.example.org", WebDAVEndpoint: testWebDAVRoot, ClientTimeout: 2},
			repo:        repo,
			client:      ocmd.NewClient(2*time.Second, false),
			transferrer: xfer,
			notifier: &syncNotifier{
				c:   ocmd.NewPublicOnlyClientWithConfig(ocmclient.TransportConfig{Timeout: 2 * time.Second, AllowLoopback: true}),
				key: key,
			},
		},
		repo: repo,
		peer: peer,
		xfer: xfer,
		logs: &lockedBuffer{},
	}
	logger := zerolog.New(env.logs)
	env.owner = appctx.WithLogger(appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox.example.org"},
	}), &logger)
	// The recipient is the exact identity stored as the received grantee.
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
			UserId: appctx.ContextMustGetUser(e.recip).Id,
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

type unacknowledgedTransferCompletion struct {
	share.Repository
	applied bool
}

func (r unacknowledgedTransferCompletion) CompleteReceivedShareTransfer(ctx context.Context, user *userpb.User, id *ocm.ShareId, generation uint64, state ocm.ShareState) (*share.ReceivedShareUpdate, error) {
	if r.applied {
		receipt, err := r.Repository.CompleteReceivedShareTransfer(ctx, user, id, generation, state)
		if err != nil {
			return nil, err
		}
		return receipt, &share.CommitUnknownError{Observed: share.CommitObservedRequested}
	}
	return nil, &share.CommitUnknownError{Observed: share.CommitObservedPrevious}
}

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
	update(webdav(&providerpb.ResourcePermissions{InitiateFileUpload: true}))
	notices = requireNotices(t, e.peer, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission, ocmd.NotificationShareChangePermission)
	if _, present := fileOf(notices[3])["permissions"]; present {
		t.Fatal("upload-only permissions must leave the snapshot absent")
	}
}

func TestPermissionNoticeOmitsNormalizedWebDAVRights(t *testing.T) {
	e := newEmitEnv(t)
	sh := e.outgoingOfType(t, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER)
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}
	requested, err := share.ParseWebDAVPermissions([]string{"read", "create", "update"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{Ref: ref, Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: share.NewWebDavAccessMethod(requested.GetPermissions(), nil, nil),
		},
	}}})
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		t.Fatalf("update: %v %v", res, err)
	}
	stored, err := e.repo.GetShare(e.owner, appctx.ContextMustGetUser(e.owner), ref)
	if err != nil || !stored.GetAccessMethods()[0].GetWebdavOptions().GetPermissions().GetDelete() {
		t.Fatalf("expected the legacy SQL editor projection: %v %v", stored, err)
	}
	notices := requireNotices(t, e.peer, ocmd.NotificationShareChangePermission)
	if value, present := fileOf(notices[0])["permissions"]; present {
		t.Fatalf("notice declared permissions broadened by storage: %v", value)
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
	if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_TRANSFERRING, ""); res.GetStatus().GetCode() != rpc.Code_CODE_INVALID_ARGUMENT {
		t.Fatalf("ordinary entry: %v", res.GetStatus())
	}
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
	cur, err := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
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
	cur, _ := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if cur.State != ocm.ShareState_SHARE_STATE_REJECTED {
		t.Fatalf("state %v", cur.State)
	}

	// Reset, start a new transfer, then the old callback arrives.
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, "")
	e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/home/dest")
	e.xfer.callback(1)(nil)
	cur, _ = e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
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
	marie := appctx.ContextMustGetUser(e.recip)
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

func TestDirectedDeclineRefusesServiceCopyUntilCompletion(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		e := newEmitEnv(t)
		checkDirectedDeclineDuringCopy(t, e, sqlite.Open(e.dbPath))
	})
	t.Run("mysql", func(t *testing.T) {
		raw := os.Getenv("REVA_TEST_MYSQL")
		if raw == "" {
			t.Skip("disposable MySQL fixture is unavailable")
		}
		at := strings.LastIndexByte(raw, '@')
		if at < 0 {
			t.Fatal("invalid REVA_TEST_MYSQL address")
		}
		host, portText, err := net.SplitHostPort(raw[at+1:])
		if err != nil {
			t.Fatal("invalid REVA_TEST_MYSQL host and port")
		}
		port, err := strconv.Atoi(portText)
		user, password, ok := strings.Cut(raw[:at], ":")
		if err != nil || !ok {
			t.Fatal("invalid REVA_TEST_MYSQL credentials or port")
		}
		target := mysql.NewConfig()
		target.User, target.Passwd, target.Net, target.Addr = user, password, "tcp", raw[at+1:]
		admin, err := sql.Open("mysql", target.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = admin.Close() })
		name := "ocm_copy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP DATABASE " + name); err != nil {
				t.Errorf("drop disposable schema: %v", err)
			}
		})
		e := newEmitEnvWithDatabase(t, map[string]any{
			"db_engine": "mysql", "db_username": user, "db_password": password,
			"db_host": host, "db_port": port, "db_name": name,
		})
		target.DBName, target.ParseTime = name, true
		checkDirectedDeclineDuringCopy(t, e, gormmysql.Open(target.FormatDSN()))
	})
}

func checkDirectedDeclineDuringCopy(t *testing.T, e *emitEnv, dialect gorm.Dialector) {
	t.Helper()
	gw, peers := mountEmbeddedGateway(t, e, map[string]map[string]any{"jwt": {"secret": "embedded-route-test"}})
	db, err := gorm.Open(dialect, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	type projection struct {
		row       model.OcmReceivedShare
		protocols []model.OcmReceivedShareProtocol
	}
	snapshot := func(t *testing.T, id *ocm.ShareId) projection {
		t.Helper()
		var p projection
		if err := db.Where("id = ?", id.OpaqueId).Take(&p.row).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Where("ocm_received_share_id = ?", p.row.ID).Order("id").Find(&p.protocols).Error; err != nil {
			t.Fatal(err)
		}
		return p
	}

	recipient := appctx.ContextMustGetUser(e.recip)
	recipient.Id.Type = userpb.UserType_USER_TYPE_PRIMARY
	other := e.received(t, "directed-first", false)
	rs := e.received(t, "directed-copy", true)
	sender := &userpb.UserId{OpaqueId: "alice", Idp: e.peer.host()}
	actions := e.repo.(share.DirectedShareActions)
	update := func(t *testing.T, id *ocm.ShareId, state ocm.ShareState, hidden bool, want rpc.Code, paths ...string) {
		t.Helper()
		res, err := gw.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
			Share:      &ocm.ReceivedShare{Id: id, State: state, Destination: "/home/dest", Hidden: hidden},
			UpdateMask: &field_mask.FieldMask{Paths: paths},
		})
		if err != nil || res.GetStatus() == nil || res.GetStatus().GetCode() != want {
			t.Fatalf("update %v to %v: status %v, error %v, want %v", paths, state, res.GetStatus(), err, want)
		}
	}

	beforeOther := snapshot(t, other.Id)
	beforeCopy := snapshot(t, rs.Id)
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_TRANSFERRING, true, rpc.Code_CODE_INVALID_ARGUMENT, "state", "hidden")
	if !reflect.DeepEqual(beforeCopy, snapshot(t, rs.Id)) {
		t.Fatal("refused entry changed the stored projection")
	}

	started, finished := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	e.xfer.park = func(done func(error)) {
		close(started)
		<-release
		done(nil)
		close(finished)
	}
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the controlled copy")
		}
	}
	t.Cleanup(func() {
		finish()
		if len(e.xfer.callbacks) > 0 {
			wait(finished)
		}
	})
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_OK, "state")
	wait(started)
	beforeCopy = snapshot(t, rs.Id)
	gen := beforeCopy.row.TransferGeneration
	if beforeCopy.row.State != model.OcmShareStateTransferring || gen == 0 {
		t.Fatal("copy started without a reservation")
	}
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, false, rpc.Code_CODE_ABORTED, "state")
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_REJECTED, false, rpc.Code_CODE_ABORTED, "state")
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_REJECTED, true, rpc.Code_CODE_ABORTED, "state", "hidden")
	if u, err := e.repo.CompleteReceivedShareTransfer(e.recip, recipient, rs.Id, gen+1, ocm.ShareState_SHARE_STATE_ACCEPTED); err != nil ||
		u == nil || u.After.GetState() != ocm.ShareState_SHARE_STATE_TRANSFERRING || !proto.Equal(u.Before, u.After) {
		t.Fatalf("wrong-generation completion receipt %v, error %v", u, err)
	}
	if !reflect.DeepEqual(beforeCopy, snapshot(t, rs.Id)) || !reflect.DeepEqual(beforeOther, snapshot(t, other.Id)) {
		t.Fatal("refused state changes altered a stored projection")
	}

	updates, err := actions.RejectReceivedOCMSharesFromSender(e.recip, recipient.Id, sender)
	if updates != nil || !errors.Is(err, share.ErrTransferInProgress) {
		t.Fatalf("directed decline returned %d receipts and %v", len(updates), err)
	}
	if !reflect.DeepEqual(beforeCopy, snapshot(t, rs.Id)) || !reflect.DeepEqual(beforeOther, snapshot(t, other.Id)) {
		t.Fatal("refused batch changed a stored projection")
	}
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_FAILED_PRECONDITION, "state")
	update(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, true, rpc.Code_CODE_OK, "hidden")
	got := snapshot(t, rs.Id)
	if !got.row.Hidden || got.row.State != model.OcmShareStateTransferring || got.row.TransferGeneration != gen ||
		!reflect.DeepEqual(beforeCopy.protocols, got.protocols) {
		t.Fatal("hidden-only update did not preserve the reservation and protocols")
	}
	update(t, other.Id, ocm.ShareState_SHARE_STATE_PENDING, true, rpc.Code_CODE_OK, "hidden")
	if got := snapshot(t, other.Id).row; !got.Hidden || got.State != model.OcmShareStatePending {
		t.Fatal("unrelated masked update did not apply")
	}
	if len(e.xfer.callbacks) != 1 || peers.datatxCalls != 0 || len(e.svc.notifier.(*syncNotifier).results) != 0 {
		t.Fatal("a refused operation started another copy or attempted a peer notice")
	}
	requireNotices(t, e.peer)
	finish()
	wait(finished)
	notices := requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	if fileOf(notices[0])["providerId"] != rs.RemoteShareId {
		t.Fatal("completion announced the wrong grant")
	}
	e.xfer.callback(0)(nil)
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	if got := snapshot(t, rs.Id).row; got.State != model.OcmShareStateAccepted || got.TransferGeneration != gen || !got.Hidden {
		t.Fatal("owner completion did not preserve the generation and hidden flag")
	}

	updates, err = actions.RejectReceivedOCMSharesFromSender(e.recip, recipient.Id, sender)
	if err != nil || len(updates) != 2 {
		t.Fatalf("directed decline after completion returned %d receipts and %v", len(updates), err)
	}
	for i, want := range []*ocm.ReceivedShare{other, rs} {
		u := updates[i]
		before := []ocm.ShareState{ocm.ShareState_SHARE_STATE_PENDING, ocm.ShareState_SHARE_STATE_ACCEPTED}[i]
		if !proto.Equal(u.Before.GetId(), want.Id) || !proto.Equal(u.After.GetId(), want.Id) ||
			u.Before.GetState() != before || u.After.GetState() != ocm.ShareState_SHARE_STATE_REJECTED || !u.After.GetHidden() {
			t.Fatalf("unexpected decline receipt %d for %s", i, want.Id.OpaqueId)
		}
	}
	update(t, other.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_OK, "state")
	update(t, other.Id, ocm.ShareState_SHARE_STATE_PENDING, false, rpc.Code_CODE_OK, "state")
	if got := snapshot(t, other.Id).row.State; got != model.OcmShareStatePending {
		t.Fatal("ordinary access reset did not persist")
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted)

	for _, tc := range []struct {
		name    string
		applied bool
		state   model.OcmShareState
	}{
		{"unapplied terminal commit", false, model.OcmShareStateTransferring},
		{"applied terminal commit", true, model.OcmShareStateAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := e.received(t, tc.name, true)
			starts := len(e.xfer.callbacks)
			update(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_OK, "state")
			reservation := snapshot(t, rs.Id)
			e.svc.repo = unacknowledgedTransferCompletion{Repository: e.repo, applied: tc.applied}
			t.Cleanup(func() { e.svc.repo = e.repo })
			e.xfer.callback(starts)(nil)
			stored := snapshot(t, rs.Id)
			if stored.row.State != tc.state || stored.row.TransferGeneration != reservation.row.TransferGeneration ||
				!reflect.DeepEqual(stored.protocols, reservation.protocols) {
				t.Fatalf("unknown completion changed the reservation unexpectedly: %v", stored.row.State)
			}
			if !tc.applied {
				update(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING, true, rpc.Code_CODE_ABORTED, "state", "hidden")
				if !reflect.DeepEqual(reservation, snapshot(t, rs.Id)) {
					t.Fatal("unresolved reservation changed")
				}
			}
			update(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_FAILED_PRECONDITION, "state")
			if len(e.xfer.callbacks) != starts+1 || peers.datatxCalls != 0 || len(e.svc.notifier.(*syncNotifier).results) != 2 {
				t.Fatal("unknown completion triggered a second dispatch or notice")
			}
			requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted)
		})
	}

	failed := e.received(t, "no dispatch", true)
	starts := len(e.xfer.callbacks)
	e.xfer.fail = errors.New("cannot dispatch copy")
	update(t, failed.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, false, rpc.Code_CODE_INTERNAL, "state")
	rejected := snapshot(t, failed.Id)
	if rejected.row.State != model.OcmShareStateRejected || rejected.row.TransferGeneration == 0 || len(e.xfer.callbacks) != starts {
		t.Fatal("synchronous no-dispatch failure did not complete its reservation")
	}
	u, err := e.repo.CompleteReceivedShareTransfer(e.recip, recipient, failed.Id, rejected.row.TransferGeneration, ocm.ShareState_SHARE_STATE_ACCEPTED)
	if err != nil || u == nil || u.StateChanged() || !reflect.DeepEqual(rejected, snapshot(t, failed.Id)) {
		t.Fatalf("completed generation changed the failed projection: %v", err)
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted)
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
				if err != nil || proto.Equal(got, before) || len(changedWebDAVPermissionNames(got, nil)) != 2 {
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
	cur, err := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
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
	sender := e.svc.notifier.(*syncNotifier)
	a := &ocmd.AsyncNotifier{Client: sender.c, Key: sender.key}
	e.svc.notifier = a
	t.Cleanup(func() { _ = a.Close() })
	sh := e.outgoing(t)
	notice, err := (ocmd.ShareStateNotification{
		Type:         ocmd.NotificationShareUnshared,
		SenderDomain: e.svc.conf.ProviderDomain,
		ResourceType: "file",
		ShareType:    "user",
		ProviderID:   sh.Id.OpaqueId,
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := a.NewNotification(e.owner, e.peer.host(), notice), (ocmd.SendResult{Outcome: ocmd.SendPostFailed, Failure: ocmd.FailureCanceled}); got != want {
		t.Fatalf("post-close admission = %+v, want %+v", got, want)
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

type deadlineRepo struct {
	share.Repository
	check func(context.Context) (*ocm.Share, error)
}

func (r deadlineRepo) GetShare(ctx context.Context, _ *userpb.User, _ *ocm.ShareReference) (*ocm.Share, error) {
	return r.check(ctx)
}

type notificationFunc func(context.Context, string, *ocmd.OutgoingNotification) ocmd.SendResult

func (f notificationFunc) NewNotification(ctx context.Context, domain string, n *ocmd.OutgoingNotification) ocmd.SendResult {
	return f(ctx, domain, n)
}

func TestNotificationFreshnessUsesEffectiveClientTimeout(t *testing.T) {
	const driver = "notification-timeout-test"
	var repo deadlineRepo
	withTestDrivers(t, driver, func(context.Context, map[string]any) (share.Repository, error) {
		return repo, nil
	}, func(context.Context, map[string]any) (embedded.Transferrer, error) {
		return noopTransferrer{}, nil
	})

	defaultTimeout := ocmclient.DefaultTransportConfig().Timeout
	for _, tc := range []struct {
		configured int
		want       time.Duration
	}{{-1, defaultTimeout}, {0, defaultTimeout}, {3, 3 * time.Second}} {
		t.Run(strconv.Itoa(tc.configured), func(t *testing.T) {
			sh := &ocm.Share{
				Id:            &ocm.ShareId{OpaqueId: "grant"},
				Grantee:       &providerpb.Grantee{Id: &providerpb.Grantee_UserId{UserId: &userpb.UserId{Idp: "peer.example.org"}}},
				AccessMethods: []*ocm.AccessMethod{share.NewWebDavAccessMethod(&providerpb.ResourcePermissions{}, nil, nil)},
			}
			var checked bool
			repo.check = func(ctx context.Context) (*ocm.Share, error) {
				checked = true
				deadline, ok := ctx.Deadline()
				if remaining := time.Until(deadline); !ok || remaining <= tc.want-time.Second || remaining > tc.want {
					t.Errorf("freshness deadline has %v remaining, want %v", remaining, tc.want)
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return sh, nil
			}
			ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "owner"}})
			raw, err := New(ctx, map[string]any{
				"driver": driver, "embedded_driver": driver, "gatewaysvc": "127.0.0.1:9142",
				"provider_domain": "sender.example", "webdav_endpoint": testWebDAVRoot,
				"signing_key": writeKeyFile(t), "client_timeout": tc.configured,
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := raw.(*service)
			t.Cleanup(func() { _ = svc.Close() })
			svc.notifier = notificationFunc(func(ctx context.Context, _ string, n *ocmd.OutgoingNotification) ocmd.SendResult {
				if n.StillCurrent == nil || !n.StillCurrent(ctx) {
					t.Error("current snapshot failed freshness check")
				}
				return ocmd.SendResult{}
			})
			svc.notifyPermissionChange(ctx, &share.ShareUpdate{
				After: sh, ResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE, WebDAVPermissionsChanged: true,
			}, nil)
			if !checked || time.Duration(svc.conf.ClientTimeout)*time.Second != tc.want {
				t.Fatalf("timeout = %d seconds, repository read = %t", svc.conf.ClientTimeout, checked)
			}
		})
	}
}

func TestAsyncJSONPermissionNoticeChecksFreshnessDeadline(t *testing.T) {
	for _, name := range []string{"current", "expired"} {
		t.Run(name, func(t *testing.T) {
			e := newEmitEnv(t)
			repo, err := jsonrepo.New(t.Context(), map[string]any{"file": filepath.Join(t.TempDir(), "shares.json")})
			if err != nil {
				t.Fatal(err)
			}
			e.repo = repo
			e.svc.conf.Driver = "json"
			e.svc.conf.ClientTimeout = 10
			if name == "expired" {
				e.svc.conf.ClientTimeout = 1
			}
			sh := e.outgoing(t)
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}
			owner := appctx.ContextMustGetUser(e.owner)
			e.svc.repo = deadlineRepo{Repository: repo, check: func(ctx context.Context) (*ocm.Share, error) {
				current, err := repo.GetShare(ctx, appctx.ContextMustGetUser(ctx), ref)
				if name == "expired" {
					<-ctx.Done()
				}
				return current, err
			}}

			sender := e.svc.notifier.(*syncNotifier)
			a := &ocmd.AsyncNotifier{Client: sender.c, Key: sender.key}
			t.Cleanup(func() { _ = a.Close() })
			checked := make(chan bool, 2)
			e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
				check := n.StillCurrent
				n.StillCurrent = func(ctx context.Context) bool {
					current := check(ctx)
					checked <- current
					return current
				}
				return a.NewNotification(ctx, peer, n)
			})
			posted := make(chan struct{})
			e.peer.mu.Lock()
			e.peer.posted = posted
			e.peer.mu.Unlock()

			empty := &providerpb.ResourcePermissions{}
			res, err := e.svc.UpdateOCMShare(e.owner, &ocm.UpdateOCMShareRequest{
				Ref: ref, Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
					Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
						AccessMethods: share.NewWebDavAccessMethod(empty, nil, nil),
					},
				}},
			})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK {
				t.Fatalf("permission update = %v, err = %v", res, err)
			}
			current, err := repo.GetShare(t.Context(), owner, ref)
			if err != nil || !slices.ContainsFunc(current.GetAccessMethods(), func(method *ocm.AccessMethod) bool {
				return proto.Equal(method.GetWebdavOptions().GetPermissions(), empty)
			}) {
				t.Fatalf("committed share = %v, err = %v", current, err)
			}
			select {
			case got := <-checked:
				if got != (name == "current") {
					t.Errorf("freshness returned %t for %s read", got, name)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("permission freshness check did not finish")
			}
			if name == "current" {
				select {
				case <-posted:
				case <-time.After(10 * time.Second):
					t.Fatal("current permission notification did not reach the peer")
				}
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			if name == "current" {
				got := requireNotices(t, e.peer, ocmd.NotificationShareChangePermission)[0]
				if perms, ok := fileOf(got)["permissions"].([]any); !ok || len(perms) != 0 {
					t.Fatalf("notification permissions = %v, want explicit empty", fileOf(got)["permissions"])
				}
			} else {
				requireNotices(t, e.peer)
				e.peer.mu.Lock()
				defer e.peer.mu.Unlock()
				if e.peer.discovers != 0 {
					t.Fatalf("expired read started %d discoveries", e.peer.discovers)
				}
			}
		})
	}
}

func TestNewNotificationClientVerifiesTLS(t *testing.T) {
	const driver = "notification-tls-test"
	withTestDrivers(t, driver, func(context.Context, map[string]any) (share.Repository, error) {
		return &capturingRepo{}, nil
	}, func(context.Context, map[string]any) (embedded.Transferrer, error) {
		return noopTransferrer{}, nil
	})

	const peer = "example.com"
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
				Enabled: true, APIVersion: "1.4.0", Endpoint: "https://" + peer + "/ocm", Capabilities: []string{"notifications"},
			})
		case "/ocm/notifications":
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	out, err := (ocmd.ShareStateNotification{
		Type: ocmd.NotificationShareAccepted, SenderDomain: "sender.example",
		ResourceType: "file", ShareType: "user", ProviderID: "grant",
	}).Build()
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, tc := range []struct {
		name    string
		trusted bool
		want    ocmd.SendOutcome
	}{{"untrusted", false, ocmd.SendDiscoveryFailed}, {"trusted", true, ocmd.SendDelivered}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			raw, err := New(ctx, map[string]any{
				"driver": driver, "embedded_driver": driver, "gatewaysvc": "127.0.0.1:9142",
				"provider_domain": "sender.example", "webdav_endpoint": testWebDAVRoot,
				"signing_key": writeKeyFile(t), "client_insecure": true,
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := raw.(*service)
			t.Cleanup(func() { _ = svc.Close() })
			if !ocmclient.HTTPTransport(svc.client.Transport()).TLSClientConfig.InsecureSkipVerify {
				t.Fatal("ordinary client lost its configured TLS setting")
			}
			n := svc.notifier.(*ocmd.AsyncNotifier)
			tr := ocmclient.HTTPTransport(n.Client.Transport())
			t.Cleanup(tr.CloseIdleConnections)
			// Route the fixture hostname directly to the loopback TLS server.
			tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr != peer+":443" {
					return nil, errors.New("unexpected TLS fixture address")
				}
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			if tc.trusted {
				tr.TLSClientConfig.RootCAs = roots
			} else {
				tr.TLSClientConfig.RootCAs = x509.NewCertPool()
			}
			defer posts.Store(0)
			if got := n.Client.NewNotification(ctx, peer, out, n.Key); got.Outcome != tc.want {
				t.Fatalf("notification outcome = %v, want %v", got, tc.want)
			}
			wantPosts := int32(0)
			if tc.trusted {
				wantPosts = 1
			}
			if got := posts.Load(); got != wantPosts {
				t.Fatalf("notification POSTs = %d, want %d", got, wantPosts)
			}
		})
	}
}

func TestReceivedStateNoticeRechecksCommittedSQLState(t *testing.T) {
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		declined = ocm.ShareState_SHARE_STATE_REJECTED
	)
	for _, tc := range []struct {
		name   string
		states []ocm.ShareState
		remove bool
		want   []string
	}{
		{"accept then decline", []ocm.ShareState{accepted, declined}, false, []string{ocmd.NotificationShareDeclined}},
		{"decline then accept", []ocm.ShareState{declined, accepted}, false, []string{ocmd.NotificationShareAccepted}},
		{"repeat acceptance", []ocm.ShareState{accepted, accepted}, false, []string{ocmd.NotificationShareAccepted}},
		{"return to acceptance", []ocm.ShareState{accepted, declined, accepted}, false, []string{ocmd.NotificationShareAccepted, ocmd.NotificationShareAccepted}},
		{"deleted before admission", []ocm.ShareState{accepted}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			rs := e.received(t, "delayed-state", false)
			user := appctx.ContextMustGetUser(e.recip)
			var receipts []*share.ReceivedShareUpdate
			for _, state := range tc.states {
				u, err := e.repo.UpdateReceivedShare(e.recip, user,
					&ocm.ReceivedShare{Id: rs.Id, State: state}, &field_mask.FieldMask{Paths: []string{"state"}})
				if err != nil {
					t.Fatal(err)
				}
				receipts = append(receipts, u)
			}
			if tc.remove {
				db, err := gorm.Open(sqlite.Open(e.dbPath), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				pool, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = pool.Close() })
				if err := db.Where("id = ?", rs.Id.OpaqueId).Delete(&model.OcmReceivedShare{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			// Earlier callers resume after the latest change and notice.
			for _, receipt := range slices.Backward(receipts) {
				e.svc.notifyReceivedState(e.recip, receipt, true)
			}
			requireNotices(t, e.peer, tc.want...)

			current, err := e.repo.GetReceivedShare(e.recip, user, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
			if tc.remove {
				if !errors.Is(err, share.ErrShareNotFound) || current != nil {
					t.Fatalf("deleted share read back as present: %v", err)
				}
			} else if err != nil || current.GetState() != tc.states[len(tc.states)-1] {
				t.Fatalf("stored state = %v, err = %v", current.GetState(), err)
			}
		})
	}
}

func TestJSONReceivedStateNoticeKeepsSingleAttempt(t *testing.T) {
	e := newEmitEnv(t)
	e.peer.mu.Lock()
	e.peer.status = http.StatusServiceUnavailable
	e.peer.mu.Unlock()
	rs, _ := e.useJSON(t, e.received(t, "json-state", false))
	repo := e.svc.repo
	user := appctx.ContextMustGetUser(e.recip)
	var attempts int
	delegate := e.svc.notifier
	e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
		attempts++
		return delegate.NewNotification(ctx, peer, n)
	})
	for range 2 {
		if res := e.setState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.GetStatus().GetCode() != rpc.Code_CODE_OK {
			t.Fatalf("acceptance status = %v", res.GetStatus())
		}
	}
	requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	if attempts != 1 {
		t.Fatalf("notification attempts = %d, want 1", attempts)
	}
	current, err := repo.GetReceivedShare(e.recip, user, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if err != nil || current.GetState() != ocm.ShareState_SHARE_STATE_ACCEPTED {
		t.Fatalf("stored state = %v, err = %v", current.GetState(), err)
	}

	datatx := e.received(t, "json-datatx", false, ocm.AccessType_ACCESS_TYPE_DATATX)
	datatx.Grantee.GetUserId().Idp = user.GetId().GetIdp()
	datatx, err = repo.StoreReceivedShare(e.recip, datatx)
	if err != nil {
		t.Fatal(err)
	}
	if res := e.setState(t, datatx.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, ""); res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		t.Fatalf("DATATX acceptance status = %v", res.GetStatus())
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: datatx.Id}}
	if current, err := repo.GetReceivedShare(e.recip, user, ref); err != nil || current.GetState() != ocm.ShareState_SHARE_STATE_ACCEPTED {
		t.Fatalf("DATATX stored state = %v, err = %v", current.GetState(), err)
	}
	if attempts != 1 {
		t.Fatalf("DATATX acceptance added another notice attempt")
	}
}

func (e *emitEnv) useJSON(t *testing.T, rs *ocm.ReceivedShare) (*ocm.ReceivedShare, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shares.json")
	repo, err := jsonrepo.New(t.Context(), map[string]any{"file": path})
	if err != nil {
		t.Fatal(err)
	}
	rs.Grantee.GetUserId().Idp = appctx.ContextMustGetUser(e.recip).Id.Idp
	rs, err = repo.StoreReceivedShare(e.recip, rs)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.repo, e.svc.conf.Driver = repo, "json"
	return rs, path
}

type parkedStateUpdateRepo struct {
	share.Repository
	committed chan<- chan struct{}
}

func (r parkedStateUpdateRepo) UpdateReceivedShare(ctx context.Context, user *userpb.User, rs *ocm.ReceivedShare, mask *field_mask.FieldMask) (*share.ReceivedShareUpdate, error) {
	update, err := r.Repository.UpdateReceivedShare(ctx, user, rs, mask)
	if err != nil {
		return nil, err
	}
	release := make(chan struct{})
	select {
	case r.committed <- release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-release:
		return update, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestJSONReceivedStateNoticeSkipsSupersededReceipt(t *testing.T) {
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		declined = ocm.ShareState_SHARE_STATE_REJECTED
	)
	for _, tc := range []struct {
		name   string
		states []ocm.ShareState
		want   string
	}{
		{"accept then decline", []ocm.ShareState{accepted, declined}, ocmd.NotificationShareDeclined},
		{"decline then accept", []ocm.ShareState{declined, accepted}, ocmd.NotificationShareAccepted},
		{"return to acceptance", []ocm.ShareState{accepted, declined, accepted}, ocmd.NotificationShareAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			user := appctx.ContextMustGetUser(e.recip)
			rs, _ := e.useJSON(t, e.received(t, "Exact-Case:state", false))
			repo := e.svc.repo
			committed := make(chan chan struct{})
			e.svc.repo = parkedStateUpdateRepo{Repository: repo, committed: committed}
			ctx, cancel := context.WithCancel(e.recip)
			var producers sync.WaitGroup
			defer func() {
				cancel()
				producers.Wait()
			}()
			sender := e.svc.notifier.(*syncNotifier)
			a := &ocmd.AsyncNotifier{Client: sender.c, Key: sender.key}
			t.Cleanup(func() { _ = a.Close() })
			finished := make(chan struct{}, len(tc.states))
			admitted := 0
			e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
				if peer != e.peer.host() {
					t.Errorf("peer = %q, want creator host and port %q", peer, e.peer.host())
				}
				admitted++
				release := n.StateNotice.Release
				n.StateNotice.Release = func() {
					release()
					finished <- struct{}{}
				}
				result := a.NewNotification(ctx, peer, n)
				if result != (ocmd.SendResult{}) {
					t.Errorf("notice admission = %+v", result)
				}
				return result
			})
			type producer struct {
				release chan struct{}
				done    chan struct{}
			}
			var parked []producer
			for _, state := range tc.states {
				done := make(chan struct{})
				producers.Go(func() {
					defer close(done)
					resp, err := e.svc.UpdateReceivedOCMShare(ctx, &ocm.UpdateReceivedOCMShareRequest{
						Share:      &ocm.ReceivedShare{Id: rs.Id, State: state},
						UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
					})
					if ctx.Err() == nil && (err != nil || resp.GetStatus().GetCode() != rpc.Code_CODE_OK) {
						t.Errorf("update to %s: status = %v, err = %v", state, resp.GetStatus(), err)
					}
				})
				select {
				case release := <-committed:
					parked = append(parked, producer{release: release, done: done})
				case <-done:
					t.Fatalf("producer for %s returned before the commit barrier", state)
				case <-time.After(5 * time.Second):
					t.Fatalf("producer for %s did not reach the commit barrier", state)
				}
			}
			// Readback can retire stale proofs on load, so let the workers check first.
			for i, p := range slices.Backward(parked) {
				close(p.release)
				select {
				case <-p.done:
				case <-time.After(5 * time.Second):
					t.Fatalf("producer %d did not return after release", i)
				}
				if i == len(parked)-1 {
					select {
					case <-finished:
					case <-time.After(5 * time.Second):
						t.Fatal("state worker did not finish while notifier was live")
					}
				}
				if admitted != 1 {
					t.Fatalf("admissions = %d at producer %d, want 1", admitted, i)
				}
				e.peer.mu.Lock()
				discoveries := e.peer.discovers
				e.peer.mu.Unlock()
				if discoveries != 1 {
					t.Fatalf("discoveries = %d at producer %d, want 1", discoveries, i)
				}
				got := requireNotices(t, e.peer, tc.want)[0]
				_, hasPermissions := fileOf(got)["permissions"]
				if got["senderDomain"] != "cernbox.example.org" || got["resourceType"] != "file" ||
					fileOf(got)["providerId"] != rs.RemoteShareId || hasPermissions {
					t.Fatalf("incorrect state notification: %v", got)
				}
			}
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
			current, err := repo.GetReceivedShare(t.Context(), user, ref)
			if err != nil || current.GetState() != tc.states[len(tc.states)-1] {
				t.Fatalf("committed state = %v, err = %v", current.GetState(), err)
			}
		})
	}
}

func TestJSONReceivedStateNoticeWhileRepositoryBlocked(t *testing.T) {
	t.Run("manager held", func(t *testing.T) {
		testJSONReceivedStateNoticeWhileRepositoryBlocked(t, false, nil)
	})
	t.Run("client timeout", func(t *testing.T) {
		testJSONReceivedStateNoticeWhileRepositoryBlocked(t, true, nil)
	})
}

func testJSONReceivedStateNoticeWhileRepositoryBlocked(t *testing.T, clientTimeout bool,
	holdRead func(string, func() (*ocm.ReceivedShare, error), *ocm.ReceivedShare, func()),
) {
	t.Helper()
	e := newEmitEnv(t)
	rs, path := e.useJSON(t, e.received(t, "held-json", false))
	repo := e.svc.repo
	user := appctx.ContextMustGetUser(e.recip)
	const accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
	receipt, err := repo.UpdateReceivedShare(share.WithStateNotice(e.recip), user,
		&ocm.ReceivedShare{Id: rs.Id, State: accepted}, &field_mask.FieldMask{Paths: []string{"state"}})
	if err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	current, err := repo.GetReceivedShare(t.Context(), user, ref)
	if err != nil || current.GetState() != accepted {
		t.Fatalf("committed state = %v, err = %v", current.GetState(), err)
	}
	posted, canceled := make(chan struct{}), make(chan struct{})
	releasePeer := make(chan struct{})
	defer close(releasePeer)
	var cancelObserved sync.Once
	e.peer.mu.Lock()
	e.peer.posted = posted
	e.peer.holdPost = func(ctx context.Context) {
		select {
		case <-ctx.Done():
			cancelObserved.Do(func() { close(canceled) })
			<-releasePeer
		case <-releasePeer:
		}
	}
	e.peer.mu.Unlock()
	timeout := time.Minute
	if clientTimeout {
		timeout = time.Second
	}
	a := &ocmd.AsyncNotifier{
		Client: ocmd.NewPublicOnlyClientWithConfig(ocmclient.TransportConfig{Timeout: timeout, AllowLoopback: true}),
		Key:    e.svc.notifier.(*syncNotifier).key,
	}
	e.svc.notifier = a
	t.Cleanup(func() { _ = a.Close() })
	wait := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}
	notify := func() {
		e.svc.notifyReceivedState(e.recip, receipt, true)
		wait(posted, "signed POST")
		if clientTimeout {
			wait(canceled, "request deadline")
		}
		closed := make(chan struct{})
		go func() {
			_ = a.Close()
			close(closed)
		}()
		wait(canceled, "peer request cancellation")
		wait(closed, "notifier join")
		requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
		e.peer.mu.Lock()
		defer e.peer.mu.Unlock()
		if e.peer.discovers != 1 {
			t.Fatalf("discoveries = %d, want 1", e.peer.discovers)
		}
	}
	if holdRead != nil {
		holdRead(path, func() (*ocm.ReceivedShare, error) {
			return repo.GetReceivedShare(t.Context(), user, ref)
		}, current, notify)
		return
	}
	if !clientTimeout {
		locker := repo.(sync.Locker)
		locker.Lock()
		defer locker.Unlock()
	}
	notify()
}

func TestJSONReceivedStateNoticeRetiresUnhandedProof(t *testing.T) {
	e := newEmitEnv(t)
	rs, _ := e.useJSON(t, e.received(t, "same-stripe", false))
	user := appctx.ContextMustGetUser(e.recip)
	update := func(state ocm.ShareState) *share.ReceivedShareUpdate {
		t.Helper()
		receipt, err := e.svc.repo.UpdateReceivedShare(share.WithStateNotice(e.recip), user,
			&ocm.ReceivedShare{Id: rs.Id, State: state}, &field_mask.FieldMask{Paths: []string{"state"}})
		if err != nil || receipt.ClaimStateNotice == nil {
			t.Fatalf("transition to %s: receipt = %v, err = %v", state, receipt, err)
		}
		return receipt
	}
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		declined = ocm.ShareState_SHARE_STATE_REJECTED
	)
	for i, reason := range []string{"no notifier", "invalid peer", "missing resource type", "implicit decline", "closed notifier"} {
		receipt := update([]ocm.ShareState{accepted, declined, accepted, declined, accepted}[i])
		var claimed *share.StateNotice
		claim := receipt.ClaimStateNotice
		receipt.ClaimStateNotice = func() *share.StateNotice {
			claimed = claim()
			return claimed
		}
		e.svc.notifier = notificationFunc(func(context.Context, string, *ocmd.OutgoingNotification) ocmd.SendResult {
			t.Fatalf("%s reached notifier admission", reason)
			return ocmd.SendResult{}
		})
		switch reason {
		case "no notifier":
			e.svc.notifier = nil
		case "invalid peer":
			receipt.After.Creator.Idp = "https://invalid.example"
		case "missing resource type":
			receipt.After.SharedResourceType = ocm.SharedResourceType(0)
		case "closed notifier":
			var closed ocmd.AsyncNotifier
			_ = closed.Close()
			e.svc.notifier = &closed
		}
		e.svc.notifyReceivedState(e.recip, receipt, reason != "implicit decline")
		if claimed == nil || claimed.StillCurrent() {
			t.Fatalf("%s did not claim and retire the proof", reason)
		}
	}

	// Fill the notifier independently of the repository's proof budget.
	receipt := update(declined)
	synctest.Test(t, func(t *testing.T) {
		var a ocmd.AsyncNotifier
		release, entered, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
		defer a.Close()
		defer close(release)
		offer := func(check func(context.Context) bool) {
			t.Helper()
			n, err := (ocmd.ShareStateNotification{
				Type: ocmd.NotificationShareAccepted, SenderDomain: "sender.example",
				ResourceType: "file", ShareType: "user", ProviderID: rs.RemoteShareId,
			}).Build()
			if err != nil {
				t.Fatal(err)
			}
			n.StillCurrent = check
			if got := a.NewNotification(t.Context(), e.peer.host(), n); got != (ocmd.SendResult{}) {
				t.Fatalf("stripe admission = %+v", got)
			}
		}
		offer(func(context.Context) bool {
			close(entered)
			<-release
			return false
		})
		<-entered
		for range 63 {
			offer(func(context.Context) bool {
				t.Error("same-stripe waiter reached eligibility")
				return false
			})
		}
		synctest.Wait()
		var refused *share.StateNotice
		e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
			refused = n.StateNotice
			return a.NewNotification(ctx, peer, n)
		})
		e.svc.notifyReceivedState(e.recip, receipt, true)
		if refused == nil || refused.StillCurrent() {
			t.Error("saturated admission left its proof live")
		}
		go func() {
			_ = a.Close()
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Error("Close returned with the stripe holder parked")
		default:
		}
	})
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	if current, err := e.svc.repo.GetReceivedShare(t.Context(), user, ref); err != nil || current.GetState() != declined {
		t.Fatalf("saturated notice state = %v, err = %v", current.GetState(), err)
	}
}

func TestJSONReceivedStateNoticeRechecksAroundDiscovery(t *testing.T) {
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		pending  = ocm.ShareState_SHARE_STATE_PENDING
	)
	for _, tc := range []struct {
		name               string
		discoveries, posts int
	}{
		{"reset before discovery", 0, 0},
		{"reset during discovery", 1, 0},
		{"unchanged", 1, 1},
		{"reset then accept", 2, 1},
		{"missing capability", 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			rs, _ := e.useJSON(t, e.received(t, "Exact-Case:discovery", false))
			sender := e.svc.notifier.(*syncNotifier)
			a := &ocmd.AsyncNotifier{Client: sender.c, Key: sender.key}
			t.Cleanup(func() { _ = a.Close() })
			entered, resume := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(resume) })
			defer unblock()
			wait := func(ch <-chan struct{}, stage string) {
				t.Helper()
				select {
				case <-ch:
				case <-time.After(5 * time.Second):
					t.Fatalf("timed out waiting for %s", stage)
				}
			}
			finished := make(chan struct{}, 2)
			var proof *share.StateNotice
			admissions := 0
			e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
				admissions++
				proof = n.StateNotice
				release := proof.Release
				proof.Release = func() {
					release()
					finished <- struct{}{}
				}
				result := a.NewNotification(ctx, peer, n)
				if result != (ocmd.SendResult{}) {
					t.Errorf("notice admission = %+v", result)
				}
				return result
			})

			if tc.name == "reset before discovery" {
				n := &ocmd.OutgoingNotification{File: &ocmd.NotificationFile{ProviderID: rs.RemoteShareId}}
				n.StillCurrent = func(context.Context) bool {
					close(entered)
					<-resume
					return false
				}
				if result := a.NewNotification(t.Context(), e.peer.host(), n); result != (ocmd.SendResult{}) {
					t.Fatalf("stripe holder admission = %+v", result)
				}
				wait(entered, "stripe holder")
			} else {
				e.peer.mu.Lock()
				e.peer.noCap = tc.name == "missing capability"
				e.peer.holdDiscovery = func(ctx context.Context) {
					close(entered)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				}
				e.peer.mu.Unlock()
			}
			update := func(state ocm.ShareState) {
				t.Helper()
				if res := e.setState(t, rs.Id, state, ""); res.GetStatus().GetCode() != rpc.Code_CODE_OK {
					t.Fatalf("update to %s = %v", state, res.GetStatus())
				}
			}
			update(accepted)
			if tc.name != "reset before discovery" {
				wait(entered, "peer discovery")
			}
			if proof == nil || !proof.StillCurrent() {
				t.Fatal("proof lost currentness before reset or POST")
			}
			if tc.name == "reset before discovery" || tc.name == "reset during discovery" || tc.name == "reset then accept" {
				update(pending)
				if proof.StillCurrent() {
					t.Fatal("acknowledged reset left the original proof current")
				}
				ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
				current, err := e.svc.repo.GetReceivedShare(t.Context(), appctx.ContextMustGetUser(e.recip), ref)
				if err != nil || current.GetState() != pending {
					t.Fatalf("committed reset = %v, err = %v", current, err)
				}
			}
			first := proof
			if tc.name == "reset then accept" {
				e.peer.mu.Lock()
				e.peer.holdDiscovery = nil
				e.peer.mu.Unlock()
				update(accepted)
				if proof == first || !proof.StillCurrent() {
					t.Fatal("second notice did not get an independent live proof")
				}
			}
			unblock()
			wait(finished, "first worker release")
			if first.StillCurrent() {
				t.Fatal("first worker left its proof current")
			}
			if tc.name == "reset then accept" {
				wait(finished, "second worker release")
				if proof.StillCurrent() {
					t.Fatal("second worker left its proof current")
				}
			}
			wantAdmissions := 1
			if tc.name == "reset then accept" {
				wantAdmissions = 2
			}
			if admissions != wantAdmissions {
				t.Fatalf("admissions = %d, want %d", admissions, wantAdmissions)
			}
			e.peer.mu.Lock()
			discoveries := e.peer.discovers
			e.peer.mu.Unlock()
			posts, signed := e.peer.received()
			if discoveries != tc.discoveries || len(posts) != tc.posts || signed != tc.posts {
				t.Fatalf("discoveries = %d, POSTs = %d, signed = %d, want %d/%d/%d",
					discoveries, len(posts), signed, tc.discoveries, tc.posts, tc.posts)
			}
			if tc.posts != 0 {
				n := requireNotices(t, e.peer, ocmd.NotificationShareAccepted)[0]
				if fileOf(n)["providerId"] != rs.RemoteShareId {
					t.Fatalf("provider ID = %q, want %q", fileOf(n)["providerId"], rs.RemoteShareId)
				}
			}
		})
	}
}

func TestReceivedStateNoticeRechecksSQLAroundDiscovery(t *testing.T) {
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		pending  = ocm.ShareState_SHARE_STATE_PENDING
		declined = ocm.ShareState_SHARE_STATE_REJECTED
	)
	for _, tc := range []struct {
		name, notice string
		state        ocm.ShareState
		discoveries  int
	}{
		{"reset during discovery", "", pending, 1},
		{"unchanged", ocmd.NotificationShareAccepted, accepted, 1},
		{"decline during discovery", ocmd.NotificationShareDeclined, declined, 2},
		{"permissions replaced during discovery", ocmd.NotificationShareChangePermission, accepted, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEmitEnv(t)
			sender := e.svc.notifier.(*syncNotifier)
			rs := e.received(t, "Exact-Case:sql-discovery", false)
			providerID := rs.RemoteShareId
			update := func(first bool) (rpc.Code, error) {
				ctx, cancel := context.WithTimeout(e.recip, 5*time.Second)
				defer cancel()
				state := tc.state
				if first {
					state = accepted
				}
				res, err := e.svc.UpdateReceivedOCMShare(ctx, &ocm.UpdateReceivedOCMShareRequest{
					Share:      &ocm.ReceivedShare{Id: rs.Id, State: state},
					UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
				})
				return res.GetStatus().GetCode(), err
			}
			if tc.notice == ocmd.NotificationShareChangePermission {
				sh := e.outgoing(t)
				providerID = sh.Id.OpaqueId
				update = func(first bool) (rpc.Code, error) {
					ctx, cancel := context.WithTimeout(e.owner, 5*time.Second)
					defer cancel()
					p := &providerpb.ResourcePermissions{}
					if first {
						p = permissions.NewEditorRole().CS3ResourcePermissions()
					}
					res, err := e.svc.UpdateOCMShare(ctx, &ocm.UpdateOCMShareRequest{
						Ref: &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}},
						Field: []*ocm.UpdateOCMShareRequest_UpdateField{{
							Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
								AccessMethods: share.NewWebDavAccessMethod(p, nil, nil),
							},
						}},
					})
					return res.GetStatus().GetCode(), err
				}
			}

			entered, resume, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(resume) })
			t.Cleanup(func() {
				unblock()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("timed out joining first provider update in cleanup")
				}
			})
			e.peer.mu.Lock()
			e.peer.holdDiscovery = func(ctx context.Context) {
				close(entered)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}
			e.peer.mu.Unlock()

			var firstStatus rpc.Code
			var firstErr error
			go func() {
				defer close(done)
				firstStatus, firstErr = update(true)
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first update did not reach discovery")
			}
			e.peer.mu.Lock()
			e.peer.holdDiscovery = nil
			e.peer.mu.Unlock()

			if tc.notice != ocmd.NotificationShareAccepted {
				if code, err := update(false); err != nil || code != rpc.Code_CODE_OK {
					t.Fatalf("second update = %s, err = %v", code, err)
				}
			}
			unblock()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("first provider update did not finish")
			}
			if firstErr != nil || firstStatus != rpc.Code_CODE_OK {
				t.Fatalf("first update = %s, err = %v", firstStatus, firstErr)
			}

			delivered := ocmd.SendResult{Outcome: ocmd.SendDelivered, Status: http.StatusCreated, Class: http.StatusCreated}
			superseded := ocmd.SendResult{Outcome: ocmd.SendSuperseded}
			want := []ocmd.SendResult{superseded}
			if tc.notice == ocmd.NotificationShareAccepted {
				want = []ocmd.SendResult{delivered}
			} else if tc.notice != "" {
				want = []ocmd.SendResult{delivered, superseded}
			}
			if !slices.Equal(sender.results, want) {
				t.Errorf("send results = %+v, want %+v", sender.results, want)
			}
			e.peer.mu.Lock()
			discoveries := e.peer.discovers
			e.peer.mu.Unlock()
			if discoveries != tc.discoveries {
				t.Errorf("discoveries = %d, want %d", discoveries, tc.discoveries)
			}
			if tc.notice == "" {
				requireNotices(t, e.peer)
				return
			}
			file := fileOf(requireNotices(t, e.peer, tc.notice)[0])
			if file["providerId"] != providerID {
				t.Errorf("provider ID = %q, want %q", file["providerId"], providerID)
			}
			if tc.notice == ocmd.NotificationShareChangePermission {
				if perms, ok := file["permissions"].([]any); !ok || len(perms) != 0 {
					t.Errorf("permissions = %v, want explicit empty", file["permissions"])
				}
			}
		})
	}
}

func TestJSONReceivedStateNoticeReleasesCanceledStripeWaiter(t *testing.T) {
	e := newEmitEnv(t)
	rs, _ := e.useJSON(t, e.received(t, "held-stripe", false))
	receipt, err := e.svc.repo.UpdateReceivedShare(share.WithStateNotice(e.recip), appctx.ContextMustGetUser(e.recip),
		&ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}, &field_mask.FieldMask{Paths: []string{"state"}})
	if err != nil || receipt.ClaimStateNotice == nil {
		t.Fatalf("acceptance receipt = %v, err = %v", receipt, err)
	}
	synctest.Test(t, func(t *testing.T) {
		var a ocmd.AsyncNotifier
		held, resume, finished, closed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
		defer a.Close()
		defer close(resume)
		host := e.peer.host()
		holder := &ocmd.OutgoingNotification{File: &ocmd.NotificationFile{ProviderID: rs.RemoteShareId}}
		holder.StillCurrent = func(context.Context) bool {
			close(held)
			<-resume
			return false
		}
		if res := a.NewNotification(t.Context(), host, holder); res != (ocmd.SendResult{}) {
			t.Fatalf("stripe holder admission = %+v", res)
		}
		<-held
		var current func() bool
		checked := false
		admissions := 0
		e.svc.notifier = notificationFunc(func(ctx context.Context, peer string, n *ocmd.OutgoingNotification) ocmd.SendResult {
			admissions++
			current = n.StateNotice.StillCurrent
			n.StateNotice.StillCurrent = func() bool {
				checked = true
				return current()
			}
			release := n.StateNotice.Release
			n.StateNotice.Release = func() {
				release()
				close(finished)
			}
			res := a.NewNotification(ctx, peer, n)
			if res != (ocmd.SendResult{}) {
				t.Errorf("state notice admission = %+v", res)
			}
			return res
		})
		e.svc.notifyReceivedState(t.Context(), receipt, true)
		e.svc.notifyReceivedState(t.Context(), receipt, true)
		synctest.Wait()
		if admissions != 1 || current == nil || !current() || checked {
			t.Fatalf("waiting proof was not live before freshness: admissions = %d, checked = %t", admissions, checked)
		}
		go func() {
			_ = a.Close()
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-finished:
			if current() || checked {
				t.Errorf("canceled proof current = %t, checked = %t", current(), checked)
			}
		default:
			t.Error("canceled stripe waiter did not release its proof")
		}
		select {
		case <-closed:
			t.Error("notifier closed while stripe holder was parked")
		default:
		}
	})
}
