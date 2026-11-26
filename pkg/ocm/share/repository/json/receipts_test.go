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

package json

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/utils"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/proto"
)

func receivedFixture(t *testing.T, m share.Repository) (*userpb.User, *ocm.ReceivedShare) {
	t.Helper()
	recipient := &userpb.User{Id: testGrantee().GetUserId()}
	rs, err := m.StoreReceivedShare(context.Background(), &ocm.ReceivedShare{
		RemoteShareId: "remote-1",
		Name:          "doc",
		Grantee:       testGrantee(),
		Owner:         &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"},
		Creator:       &userpb.UserId{OpaqueId: "alice", Idp: "provider.example.org"},
		State:         ocm.ShareState_SHARE_STATE_PENDING,
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
	})
	if err != nil {
		t.Fatal(err)
	}
	return recipient, rs
}

func mask(paths ...string) *field_mask.FieldMask { return &field_mask.FieldMask{Paths: paths} }

func TestJSONStateNoticeReconcilesAcknowledgedState(t *testing.T) {
	const (
		accepted = ocm.ShareState_SHARE_STATE_ACCEPTED
		declined = ocm.ShareState_SHARE_STATE_REJECTED
		pending  = ocm.ShareState_SHARE_STATE_PENDING
	)
	for _, tc := range []struct {
		name string
		live bool
	}{
		{"no-op", true}, {"validation error", true}, {"reset", false},
		{"transfer", false}, {"unrelated ingress", true}, {"unobserved writer", true},
		{"observed writer", false}, {"new grantee", false}, {"new creator", false},
		{"different remote ID case", false}, {"different local ID", false},
		{"deleted row", false}, {"missing file", false}, {"corrupt file", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, cleanup := setup(t)
			defer cleanup()
			m := repo.(*mgr)
			ctx := t.Context()
			user, rs := receivedFixture(t, m)
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
			update := func(repo share.Repository, ctx context.Context, state ocm.ShareState, paths ...string) (*share.ReceivedShareUpdate, error) {
				return repo.UpdateReceivedShare(ctx, user, &ocm.ReceivedShare{Id: rs.Id, State: state}, mask(paths...))
			}
			proof, err := update(m, share.WithStateNotice(ctx), declined, "state")
			if err != nil || proof.TryStateNotice == nil {
				t.Fatalf("decline receipt = %v, err = %v", proof, err)
			}
			switch tc.name {
			case "no-op":
				receipt, err := update(m, share.WithStateNotice(ctx), declined, "state")
				if err != nil || receipt.StateChanged() || receipt.TryStateNotice != nil {
					t.Fatalf("same-state update = %v, err = %v", receipt, err)
				}
			case "validation error":
				if receipt, err := update(m, share.WithStateNotice(ctx), accepted, "state", "invalid"); err == nil || receipt != nil {
					t.Fatalf("invalid update = %v, err = %v", receipt, err)
				}
			case "reset":
				receipt, err := update(m, share.WithStateNotice(ctx), pending, "state")
				if err != nil || !receipt.StateChanged() || receipt.TryStateNotice != nil {
					t.Fatalf("reset = %v, err = %v", receipt, err)
				}
			case "transfer":
				start, err := m.StartReceivedShareTransfer(ctx, user, rs, mask("state"))
				if err != nil {
					t.Fatal(err)
				}
				stale, err := m.CompleteReceivedShareTransfer(share.WithStateNotice(ctx), user, rs.Id, start.Generation-1, accepted)
				if err != nil || stale.StateChanged() || stale.TryStateNotice != nil {
					t.Fatalf("stale completion = %v, err = %v", stale, err)
				}
				failed, err := m.CompleteReceivedShareTransfer(ctx, user, rs.Id, start.Generation, declined)
				if err != nil || !failed.StateChanged() || failed.TryStateNotice != nil {
					t.Fatalf("failed completion = %v, err = %v", failed, err)
				}
				start, err = m.StartReceivedShareTransfer(ctx, user, rs, mask("state"))
				if err != nil {
					t.Fatal(err)
				}
				completed, err := m.CompleteReceivedShareTransfer(share.WithStateNotice(ctx), user, rs.Id, start.Generation, accepted)
				if err != nil || completed.TryStateNotice == nil || !completed.TryStateNotice() {
					t.Fatalf("successful completion = %v, err = %v", completed, err)
				}
			case "unrelated ingress", "unobserved writer", "observed writer":
				other, err := New(ctx, map[string]any{"file": m.c.File})
				if err != nil {
					t.Fatal(err)
				}
				receivedFixture(t, other)
				if tc.name != "unrelated ingress" {
					receipt, err := update(other, ctx, accepted, "state")
					if err != nil || !receipt.StateChanged() || receipt.TryStateNotice != nil {
						t.Fatalf("external update = %v, err = %v", receipt, err)
					}
				}
				if tc.name != "unobserved writer" {
					current, err := m.GetReceivedShare(ctx, user, ref)
					want := declined
					if tc.name == "observed writer" {
						want = accepted
					}
					if err != nil || current.GetState() != want {
						t.Fatalf("issuer read = %v, err = %v", current, err)
					}
				}
			default:
				original, err := os.ReadFile(m.c.File)
				if err != nil {
					t.Fatal(err)
				}
				switch tc.name {
				case "missing file":
					err = os.Remove(m.c.File)
				case "corrupt file":
					err = os.WriteFile(m.c.File, []byte("{"), 0600)
				default:
					var model shareModel
					if err := json.Unmarshal(original, &model); err != nil {
						t.Fatal(err)
					}
					row := model.ReceivedShares[rs.Id.OpaqueId]
					switch tc.name {
					case "new grantee":
						row.Grantee.GetUserId().Idp += ":443"
					case "new creator":
						row.Creator.Idp += ":443"
					case "different remote ID case":
						row.RemoteShareId = "Remote-1"
					case "different local ID":
						row.Id.OpaqueId += "-replaced"
					case "deleted row":
						delete(model.ReceivedShares, rs.Id.OpaqueId)
					}
					changed, err := json.Marshal(&model)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(m.c.File, changed, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				_, err = m.GetReceivedShare(ctx, user, ref)
				if (tc.name == "missing file" || tc.name == "corrupt file") && err == nil {
					t.Fatal("invalid file was readable")
				}
				if err := os.WriteFile(m.c.File, original, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := m.GetReceivedShare(ctx, user, ref); err != nil {
					t.Fatalf("restored row: %v", err)
				}
			}
			if got := proof.TryStateNotice(); got != tc.live {
				t.Errorf("earlier proof = %t, want %t", got, tc.live)
			}
			if proof.TryStateNotice() {
				t.Error("proof could be used twice")
			}
		})
	}
}

func TestJSONStateNoticeCapacityKeepsIndependentGrants(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := share.WithStateNotice(t.Context())
	accept := func(user *userpb.User, rs *ocm.ReceivedShare) *share.ReceivedShareUpdate {
		t.Helper()
		receipt, err := m.UpdateReceivedShare(ctx, user,
			&ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}, mask("state"))
		if err != nil || !receipt.StateChanged() || receipt.TryStateNotice == nil {
			t.Fatalf("accept = %v, err = %v", receipt, err)
		}
		return receipt
	}
	var parked []*share.ReceivedShareUpdate
	for range 64 {
		user, rs := receivedFixture(t, m)
		parked = append(parked, accept(user, rs))
	}
	user, rs := receivedFixture(t, m)
	refused := accept(user, rs)
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	if current, err := m.GetReceivedShare(ctx, user, ref); err != nil || current.GetState() != ocm.ShareState_SHARE_STATE_ACCEPTED {
		t.Fatalf("refused grant state = %v, err = %v", current.GetState(), err)
	}
	if refused.TryStateNotice() || !parked[0].TryStateNotice() {
		t.Fatal("refused proof was usable or displaced an earlier grant")
	}
	next := accept(receivedFixture(t, m))
	if parked[0].TryStateNotice() || !next.TryStateNotice() || refused.TryStateNotice() {
		t.Fatal("slot reuse rearmed an old receipt or lost the new one")
	}
	for i, proof := range parked[1:] {
		if !proof.TryStateNotice() {
			t.Errorf("independent grant %d lost its proof", i+1)
		}
	}
}

func TestJSONStateNoticeRevokedOnUnacknowledgedSave(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("requires mkfifo to observe bytes before a failing sync")
	}
	repo, cleanup := setup(t)
	defer cleanup()
	m := repo.(*mgr)
	ctx := share.WithStateNotice(t.Context())
	user, rs := receivedFixture(t, m)
	update := func(state ocm.ShareState) (*share.ReceivedShareUpdate, error) {
		return m.UpdateReceivedShare(ctx, user, &ocm.ReceivedShare{Id: rs.Id, State: state}, mask("state"))
	}
	earlier, err := update(ocm.ShareState_SHARE_STATE_ACCEPTED)
	if err != nil || earlier.TryStateNotice == nil {
		t.Fatalf("accepted receipt = %v, err = %v", earlier, err)
	}
	path := m.c.File
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(mkfifo, path).CombinedOutput(); err != nil {
		t.Fatalf("create read barrier: %v: %s", err, out)
	}
	result := make(chan error, 1)
	go func() {
		receipt, err := update(ocm.ShareState_SHARE_STATE_REJECTED)
		if receipt != nil {
			t.Error("unacknowledged update returned a receipt")
		}
		result <- err
	}()
	feed, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer feed.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(mkfifo, path).CombinedOutput(); err != nil {
		t.Fatalf("create save barrier: %v: %s", err, out)
	}
	saved := make(chan []byte, 1)
	go func() {
		d, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read attempted save: %v", err)
		}
		saved <- d
	}()
	if _, err := feed.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := feed.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, syscall.EINVAL) {
			t.Fatalf("save error = %v, want FIFO sync failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save did not finish")
	}
	var attempted shareModel
	select {
	case d := <-saved:
		if err := json.Unmarshal(d, &attempted); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save bytes were not observed")
	}
	if got := attempted.ReceivedShares[rs.Id.OpaqueId].GetState(); got != ocm.ShareState_SHARE_STATE_REJECTED {
		t.Fatalf("attempted state = %v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	if _, err := m.GetReceivedShare(ctx, user, ref); err != nil {
		t.Fatal(err)
	}
	if earlier.TryStateNotice() {
		t.Fatal("unacknowledged save left the earlier proof live")
	}
}

func TestJSONDeleteAndUpdateShareReceipts(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	u := testUser()
	s := testShare("tok", legacyMethods())
	if _, err := m.StoreShare(ctx, s, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER); err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: s.Id}}
	// The resource type is persisted with the share.
	m, err := New(ctx, map[string]any{"file": m.(*mgr).c.File})
	if err != nil {
		t.Fatal(err)
	}

	exp := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{
		Expiration: &typespb.Timestamp{Seconds: 1},
	}}
	if up, err := m.UpdateShare(ctx, u, ref, exp); err != nil || up.PermissionsChanged() {
		t.Fatalf("expiration only: %v %v", up, err)
	}
	editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
		AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
	}}
	if up, err := m.UpdateShare(ctx, u, ref, editor); err != nil || !up.WebDAVPermissionsChanged || up.WebappPermissionsChanged ||
		up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("webdav change: %v %v", up, err)
	}
	if up, err := m.UpdateShare(ctx, u, ref, editor); err != nil || up.PermissionsChanged() {
		t.Fatalf("repeat: %v %v", up, err)
	}

	removal, err := m.DeleteShare(ctx, u, ref)
	if err != nil || len(removal) != 1 || removal[0].Share.GetToken() != "tok" || removal[0].Share.GetGrantee() == nil ||
		removal[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER {
		t.Fatalf("delete: %v %v", removal, err)
	}
	if r, err := m.DeleteShare(ctx, u, ref); err == nil || r != nil {
		t.Fatalf("repeat delete: %v %v", r, err)
	}
}

func TestJSONFreshnessWaitDoesNotBlockNotifierClose(t *testing.T) {
	repo, cleanup := setup(t)
	defer cleanup()
	m := repo.(*mgr)
	sh := testShare("tok", nil)
	if _, err := m.StoreShare(t.Context(), sh, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE); err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: sh.Id}}

	synctest.Test(t, func(t *testing.T) {
		if err := m.gate.Acquire(t.Context(), 1); err != nil {
			t.Fatal(err)
		}
		defer m.gate.Release(1)

		for _, expiry := range []bool{false, true} {
			var a ocmd.AsyncNotifier
			entered := make(chan struct{})
			read := make(chan error, 1)
			n, err := (ocmd.ShareStateNotification{
				Type: ocmd.NotificationShareChangePermission, SenderDomain: "sender.example",
				ResourceType: "file", ShareType: "user", ProviderID: sh.Id.OpaqueId, Permissions: []string{"read"},
			}).Build()
			if err != nil {
				t.Fatal(err)
			}
			n.StillCurrent = func(ctx context.Context) bool {
				if expiry {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, time.Second)
					defer cancel()
				}
				close(entered)
				_, err := m.GetShare(ctx, testUser(), ref)
				read <- err
				return false
			}
			if result := a.NewNotification(t.Context(), "peer.example", n); result != (ocmd.SendResult{}) {
				t.Fatalf("send admission: %+v", result)
			}
			<-entered
			synctest.Wait()

			want := context.Canceled
			if expiry {
				want = context.DeadlineExceeded
				if err := <-read; !errors.Is(err, want) {
					t.Errorf("GetShare with manager held = %v, want %v", err, want)
				}
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			if !expiry {
				if err := <-read; !errors.Is(err, want) {
					t.Errorf("GetShare with manager held = %v, want %v", err, want)
				}
			}
		}
	})
	if got, err := m.GetShare(t.Context(), testUser(), ref); err != nil || !proto.Equal(got, sh) {
		t.Fatalf("current share = %v, err = %v", got, err)
	}
}

func TestJSONPermissionPresenceInUpdateReceipts(t *testing.T) {
	for _, webdav := range []bool{true, false} {
		name := "webapp"
		if webdav {
			name = "webdav"
		}
		t.Run(name, func(t *testing.T) {
			m, cleanup := setup(t)
			defer cleanup()
			ctx := context.Background()
			s := testShare("tok", nil)
			if _, err := m.StoreShare(ctx, s, ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE); err != nil {
				t.Fatal(err)
			}
			ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: s.Id}}
			for _, tc := range []struct {
				name        string
				permissions *provider.ResourcePermissions
				changed     bool
			}{
				{"absent to nil", nil, false},
				{"nil to empty", &provider.ResourcePermissions{}, true},
				{"empty to populated", &provider.ResourcePermissions{GetPath: true}, true},
				{"populated to nil", nil, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					method := share.NewWebappAccessMethod(tc.permissions, nil)
					if webdav {
						method = share.NewWebDavAccessMethod(tc.permissions, nil, nil)
					}
					up, err := m.UpdateShare(ctx, testUser(), ref, &ocm.UpdateOCMShareRequest_UpdateField{
						Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{AccessMethods: method},
					})
					if err != nil || up == nil ||
						up.WebDAVPermissionsChanged != (tc.changed && webdav) ||
						up.WebappPermissionsChanged != (tc.changed && !webdav) {
						t.Fatalf("update receipt = %v, err = %v, want changed = %t", up, err, tc.changed)
					}
				})
			}
		})
	}
}

func TestJSONTransferAttemptsPersistAcrossReload(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	user, rs := receivedFixture(t, m)

	start, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state"))
	if err != nil || start.Generation != 1 || start.Share.State != ocm.ShareState_SHARE_STATE_TRANSFERRING {
		t.Fatalf("start: %v %v", start, err)
	}
	if _, err := m.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrTransferInProgress) {
		t.Fatalf("second start: %v", err)
	}

	// A new manager on the same file sees the persisted attempt.
	reopened, err := New(ctx, map[string]any{"file": m.(*mgr).c.File})
	if err != nil {
		t.Fatal(err)
	}
	checkIgnored := func(generation uint64, requested, want ocm.ShareState, wantGeneration uint64) {
		t.Helper()
		u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, generation, requested)
		if err != nil || u == nil || u.Before == nil || u.After == nil || u.StateChanged() ||
			u.After.GetState() != want || !proto.Equal(u.Before, u.After) ||
			!proto.Equal(u.After.GetId(), rs.Id) || !proto.Equal(u.After.GetGrantee(), rs.Grantee) ||
			!proto.Equal(u.After.GetCreator(), rs.Creator) || u.After.GetRemoteShareId() != rs.RemoteShareId {
			t.Fatalf("ignored completion: %v %v", u, err)
		}
		if got := reopened.(*mgr).model.TransferGenerations[rs.Id.OpaqueId]; got != wantGeneration {
			t.Fatalf("ignored completion changed generation to %d, want %d", got, wantGeneration)
		}
	}
	checkIgnored(2, ocm.ShareState_SHARE_STATE_ACCEPTED, ocm.ShareState_SHARE_STATE_TRANSFERRING, 1)
	u, err := reopened.CompleteReceivedShareTransfer(ctx, user, rs.Id, 1, ocm.ShareState_SHARE_STATE_ACCEPTED)
	if err != nil || !u.StateChanged() || u.After.State != ocm.ShareState_SHARE_STATE_ACCEPTED {
		t.Fatalf("complete: %v %v", u, err)
	}
	checkIgnored(1, ocm.ShareState_SHARE_STATE_ACCEPTED, ocm.ShareState_SHARE_STATE_ACCEPTED, 1)
	if _, err := reopened.StartReceivedShareTransfer(ctx, user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrShareAlreadyAccepted) {
		t.Fatalf("start after acceptance: %v", err)
	}

	reset := &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_PENDING}
	if u, err := reopened.UpdateReceivedShare(ctx, user, reset, mask("state")); err != nil || !u.StateChanged() {
		t.Fatalf("reset: %v %v", u, err)
	}
	if u, err := reopened.UpdateReceivedShare(ctx, user, reset, mask("state")); err != nil || u.StateChanged() {
		t.Fatalf("repeat reset: %v %v", u, err)
	}
	second, err := reopened.StartReceivedShareTransfer(ctx, user, reset, mask("state"))
	if err != nil || second.Generation <= 1 {
		t.Fatalf("second attempt: %v %v", second, err)
	}
	checkIgnored(1, ocm.ShareState_SHARE_STATE_REJECTED, ocm.ShareState_SHARE_STATE_TRANSFERRING, second.Generation)
	stranger := &userpb.User{Id: &userpb.UserId{OpaqueId: "stranger"}}
	if _, err := reopened.UpdateReceivedShare(ctx, stranger, reset, mask("state")); err == nil {
		t.Fatal("foreign update accepted")
	}
}

func TestJSONTransferGenerationExhaustion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ocm.json")
	m, err := New(context.Background(), map[string]any{"file": path})
	if err != nil {
		t.Fatal(err)
	}
	user, rs := receivedFixture(t, m)
	mm := m.(*mgr)
	mm.Lock()
	if err := mm.load(); err != nil {
		t.Fatal(err)
	}
	mm.model.TransferGenerations[rs.Id.OpaqueId] = math.MaxInt64
	if err := mm.save(); err != nil {
		t.Fatal(err)
	}
	mm.Unlock()
	if _, err := m.StartReceivedShareTransfer(context.Background(), user, &ocm.ReceivedShare{Id: rs.Id}, mask("state")); !errors.Is(err, share.ErrTransferGenerationExhausted) {
		t.Fatalf("start: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// A share without a supported resource type is never stored, and a stored
// one, e.g. in a hand-edited file, is not changed.
func TestJSONShareResourceTypeAdmissionAndExistingMutations(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := m.StoreShare(ctx, testShare("tok-none", legacyMethods()), ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID); !errors.Is(err, share.ErrInvalidResourceType) {
		t.Fatalf("store: %v", err)
	}
	if shares, err := m.ListShares(ctx, testUser(), nil); err != nil || len(shares) != 0 {
		t.Fatalf("stored: %v %v", shares, err)
	}

	s := testShare("tok", legacyMethods())
	raw, err := utils.MarshalProtoV1ToJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: s.Id}}
	editor := &ocm.UpdateOCMShareRequest_UpdateField{Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
		AccessMethods: share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), nil, nil),
	}}
	for _, tc := range []struct{ name, typeEntry string }{
		{"prior format", ""},
		{"explicit invalid type", `,"resource_types":{"` + s.Id.OpaqueId + `":0}`},
		{"unsupported type", `,"resource_types":{"` + s.Id.OpaqueId + `":99}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ocm.json")
			file := `{"shares":{"` + s.Id.OpaqueId + `":` + string(raw) + `},"received_shares":{}` + tc.typeEntry + `}`
			if err := os.WriteFile(path, []byte(file), 0600); err != nil {
				t.Fatal(err)
			}
			repo, err := New(ctx, map[string]any{"file": path})
			if err != nil {
				t.Fatal(err)
			}
			up, updateErr := repo.UpdateShare(ctx, testUser(), ref, editor)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.typeEntry == "" {
				if updateErr != nil || up == nil || !up.WebDAVPermissionsChanged ||
					up.ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID {
					t.Errorf("update of prior-format share: %v %v", up, updateErr)
				}
				var persisted shareModel
				if err := json.Unmarshal(after, &persisted); err != nil {
					t.Fatal(err)
				}
				got := persisted.Shares[s.Id.OpaqueId]
				if got == nil || !proto.Equal(webdavPermissions(got), editor.GetAccessMethods().GetWebdavOptions().GetPermissions()) {
					t.Errorf("permissions not persisted: %v", got)
				}
				if _, present := persisted.ResourceTypes[s.Id.OpaqueId]; present {
					t.Error("update filled in a resource type")
				}
				removed, err := repo.DeleteShare(ctx, testUser(), ref)
				if err != nil || len(removed) != 1 || removed[0].ResourceType != ocm.SharedResourceType_SHARE_RESOURCE_TYPE_INVALID ||
					removed[0].Share.GetId().GetOpaqueId() != s.Id.OpaqueId {
					t.Errorf("withdrawal of prior-format share: %v %v", removed, err)
				}
				if got, err := repo.GetShare(ctx, testUser(), ref); err == nil || got != nil {
					t.Errorf("withdrawn share remains available: %v %v", got, err)
				}
				return
			}
			if !errors.Is(updateErr, share.ErrInvalidResourceType) || up != nil {
				t.Errorf("corrupt update: %v %v", up, updateErr)
			}
			if removed, err := repo.DeleteShare(ctx, testUser(), ref); !errors.Is(err, share.ErrInvalidResourceType) || removed != nil {
				t.Errorf("corrupt delete: %v %v", removed, err)
			}
			afterDelete, err := os.ReadFile(path)
			if err != nil || string(after) != file || string(afterDelete) != file {
				t.Errorf("corrupt file changed: %v", err)
			}
		})
	}
}
