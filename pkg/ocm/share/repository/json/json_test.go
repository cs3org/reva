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

package json

import (
	"context"
	"os"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/permissions"
)

func setup(t *testing.T) (share.Repository, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ocm-json-test-*")
	if err != nil {
		t.Fatal(err)
	}
	path := dir + "/ocm-shares.json"

	mgr, err := New(context.Background(), map[string]any{"file": path})
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return mgr, func() { os.RemoveAll(dir) }
}

func testUser() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: "testuser",
			Type:     userpb.UserType_USER_TYPE_PRIMARY,
		},
	}
}

func testGrantee() *provider.Grantee {
	return &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{
				OpaqueId: "sharee1",
				Type:     userpb.UserType_USER_TYPE_FEDERATED,
			},
		},
	}
}

func testResourceID() *provider.ResourceId {
	return &provider.ResourceId{StorageId: "stor", OpaqueId: "res"}
}

func testShare(token string, methods []*ocm.AccessMethod) *ocm.Share {
	u := testUser()
	now := uint64(time.Now().Unix())
	return &ocm.Share{
		ResourceId:    testResourceID(),
		Name:          "test",
		Token:         token,
		Grantee:       testGrantee(),
		Owner:         u.Id,
		Creator:       u.Id,
		Ctime:         &typespb.Timestamp{Seconds: now},
		Mtime:         &typespb.Timestamp{Seconds: now},
		Expiration:    &typespb.Timestamp{Seconds: now + 86400},
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		AccessMethods: methods,
	}
}

func legacyMethods() []*ocm.AccessMethod {
	return []*ocm.AccessMethod{
		share.NewWebDavAccessMethod(permissions.NewViewerRole().CS3ResourcePermissions(), []ocm.AccessType{}, []string{}),
		share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
	}
}

func codeFlowMethods() []*ocm.AccessMethod {
	return []*ocm.AccessMethod{
		share.NewWebDavAccessMethod(
			permissions.NewViewerRole().CS3ResourcePermissions(),
			[]ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
			[]string{"must-exchange-token"},
		),
		share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
	}
}

func findWebDAV(methods []*ocm.AccessMethod) *ocm.WebDAVAccessMethod {
	for _, m := range methods {
		if wdav, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
			return wdav.WebdavOptions
		}
	}
	return nil
}

func TestStoreAndGetShareRoundTrip(t *testing.T) {
	mgr, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	user := testUser()
	s := testShare("tok1", legacyMethods())

	stored, err := mgr.StoreShare(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}
	got, err := mgr.GetShare(ctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Id.OpaqueId != stored.Id.OpaqueId {
		t.Fatalf("id mismatch: got %s, want %s", got.Id.OpaqueId, stored.Id.OpaqueId)
	}
}

func TestCodeFlowShareRoundTripsRequirementsAndAccessTypes(t *testing.T) {
	mgr, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	user := testUser()
	s := testShare("cf-tok1", codeFlowMethods())

	stored, err := mgr.StoreShare(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}
	got, err := mgr.GetShare(ctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAV(got.AccessMethods)
	if wdav == nil {
		t.Fatal("expected WebDAV access method")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements: got %v, want [must-exchange-token]", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types: got %v, want [ACCESS_TYPE_REMOTE]", wdav.AccessTypes)
	}
}

func TestUpdateSharePreservesImmutableFields(t *testing.T) {
	mgr, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	user := testUser()
	s := testShare("cf-tok2", codeFlowMethods())

	stored, err := mgr.StoreShare(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	// Update permissions only (OCS caller shape: nil Requirements)
	_, err = mgr.UpdateShare(ctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: &ocm.AccessMethod{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := mgr.GetShare(ctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAV(got.AccessMethods)
	if wdav == nil {
		t.Fatal("expected WebDAV access method after update")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements lost: got %v", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types lost: got %v", wdav.AccessTypes)
	}
	if !wdav.Permissions.InitiateFileUpload {
		t.Error("expected editor permissions after update")
	}
}

func TestUpdateShareRejectsDifferentRequirements(t *testing.T) {
	mgr, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	user := testUser()
	s := testShare("cf-tok3", codeFlowMethods())

	stored, err := mgr.StoreShare(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	_, err = mgr.UpdateShare(ctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: &ocm.AccessMethod{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions:  permissions.NewEditorRole().CS3ResourcePermissions(),
						Requirements: []string{"something-different"},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error when updating with different requirements")
	}
}

func TestUpdateShareRejectsNewRequirementsOnLegacyShare(t *testing.T) {
	mgr, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()
	user := testUser()
	s := testShare("legacy-tok", legacyMethods())

	stored, err := mgr.StoreShare(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	_, err = mgr.UpdateShare(ctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: &ocm.AccessMethod{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions:  permissions.NewEditorRole().CS3ResourcePermissions(),
						Requirements: []string{"must-exchange-token"},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error when adding requirements to a legacy share")
	}
}
