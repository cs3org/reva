package sql

import (
	"context"
	"log"
	"os"
	"sort"
	"testing"
	"time"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/permissions"
	revashare "github.com/cs3org/reva/v3/pkg/share"
	"google.golang.org/genproto/protobuf/field_mask"
)

// ===========================
// Helper functions for tests
// ===========================

// You can use testing.T, if you want to test the code without benchmarking
func setupSuiteShares(tb testing.TB) (revashare.Manager, error, func(tb testing.TB)) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbName,
	}
	mgr, err := NewShareManager(ctx, cfg)
	if err != nil {
		return nil, err, nil
	}

	// Return a function to teardown the test
	return mgr, nil, func(tb testing.TB) {
		log.Println("teardown suite")
		os.Remove(dbName)
	}
}

func getRandomFile(owner *userpb.User) *provider.ResourceInfo {
	return &provider.ResourceInfo{
		Id: &provider.ResourceId{
			StorageId: "project-b",
			OpaqueId:  "45468401564",
		},
		Type:  provider.ResourceType_RESOURCE_TYPE_FILE,
		Path:  "/eos/project/b/myfile",
		Owner: owner.Id,
		Mtime: &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Unix())},
	}
}

// Return context populated with user info
func getUserContext(id string) context.Context {
	user := &userpb.User{
		Id: &userpb.UserId{
			OpaqueId: id,
			Type:     userpb.UserType_USER_TYPE_APPLICATION,
		},
		Username:     id,
		Mail:         "myuser@cern.ch",
		MailVerified: true,
	}
	ctx := appctx.ContextSetUser(context.Background(), user)
	return ctx
}

func getUserShareGrant(shareeId, resourcetype string) *collaboration.ShareGrant {
	sharee := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{
				Type:     userpb.UserType_USER_TYPE_APPLICATION,
				OpaqueId: shareeId,
			},
		},
	}

	sharegrant := &collaboration.ShareGrant{
		Grantee: sharee,
		Permissions: &collaboration.SharePermissions{
			Permissions: permissions.OcsPermissions(1).AsCS3Permissions(), //conversions.IntTosharePerm(1, resourcetype),
		},
	}
	return sharegrant
}

func getGroupShareGrant(shareeId, resourcetype string) *collaboration.ShareGrant {
	sharee := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_GROUP,
		Id: &provider.Grantee_GroupId{
			GroupId: &groupv1beta1.GroupId{
				OpaqueId: shareeId,
			},
		},
	}
	return &collaboration.ShareGrant{
		Grantee: sharee,
		Permissions: &collaboration.SharePermissions{
			Permissions: permissions.OcsPermissions(1).AsCS3Permissions(),
		},
	}
}

// ===========================
//        Actual tests
// ===========================

func TestGetShareById(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedshare, err := mgr.GetShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedshare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Retrieved share does not match created share, expected %s, got %s", share.Id.OpaqueId, retrievedshare.Id.OpaqueId)
	}
}

func TestGetShareByKey(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	key := &collaboration.ShareKey{
		Owner:      user.Id,
		ResourceId: file.Id,
		Grantee:    sharegrant.Grantee,
	}

	retrievedshare, err := mgr.GetShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Key{
			Key: key,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedshare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Retrieved share does not match created share, expected %s, got %s", share.Id.OpaqueId, retrievedshare.Id.OpaqueId)
	}
}

func TestGetReceivedShareById(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	receivedshare, err := mgr.GetReceivedShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if receivedshare.Share.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Retrieved received share does not match created share, expected %s, got %s", share.Id.OpaqueId, receivedshare.Share.Id.OpaqueId)
	}
}

func TestGetReceivedShareByKey(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	receivingUserCtx := getUserContext("1000")
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	key := &collaboration.ShareKey{
		Owner:      user.Id,
		ResourceId: file.Id,
		Grantee:    sharegrant.Grantee,
	}

	receivedshare, err := mgr.GetReceivedShare(receivingUserCtx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Key{
			Key: key,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if receivedshare.Share.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Retrieved received share does not match created share, expected %s, got %s", share.Id.OpaqueId, receivedshare.Share.Id.OpaqueId)
	}
}

func TestDoNotCreateSameShareTwice(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	sharee := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{
				Type:     userpb.UserType_USER_TYPE_APPLICATION,
				OpaqueId: "1000",
			},
		},
	}
	sharegrant := &collaboration.ShareGrant{
		Grantee: sharee,
		Permissions: &collaboration.SharePermissions{
			Permissions: permissions.OcsPermissions(1).AsCS3Permissions(),
		},
	}
	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)

	_, err = mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.Share(userctx, file, sharegrant)
	if err == nil {
		t.Log("Creating same share succeeded, while it should have failed")
		t.FailNow()
	}
}

func TestListShares(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	res, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListShares(userctx, nil)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
		t.FailNow()
	}

	if shares[0].Id.OpaqueId != res.Id.OpaqueId {
		t.Errorf("Expected share with id %s, got %s", res.Id.OpaqueId, shares[0].Id.OpaqueId)
	}
}

func TestListReceivedShares(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	receivingUserCtx := getUserContext("1000")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	res, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	receivedShares, err := mgr.ListReceivedShares(receivingUserCtx, nil)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(receivedShares) != 1 {
		t.Errorf("Expected 1 received share, got %d", len(receivedShares))
		t.FailNow()
	}

	if receivedShares[0].Share.Id.OpaqueId != res.Id.OpaqueId {
		t.Errorf("Expected share with id %s, got %s", res.Id.OpaqueId, receivedShares[0].Share.Id.OpaqueId)
	}
}

func TestListSharesWithFilters(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	res, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	filters := []*collaboration.Filter{
		{
			Type: collaboration.Filter_TYPE_RESOURCE_ID,
			Term: &collaboration.Filter_ResourceId{
				ResourceId: &provider.ResourceId{
					StorageId: "project-b",
					OpaqueId:  "45468401564",
				},
			},
		},
	}

	shares, err := mgr.ListShares(userctx, filters)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
	}

	if shares[0].Id.OpaqueId != res.Id.OpaqueId {
		t.Errorf("Expected share with id %s, got %s", res.Id.OpaqueId, shares[0].Id.OpaqueId)
	}
}

func TestDeleteShare(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	err = mgr.Unshare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.GetShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err == nil {
		t.Errorf("Expected share to be deleted, but it was not")
	}
}

func TestUpdateShare(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	newPermissions := 15

	updatedShare, err := mgr.UpdateShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	}, &collaboration.UpdateShareRequest{
		Field: &collaboration.UpdateShareRequest_UpdateField{
			Field: &collaboration.UpdateShareRequest_UpdateField_Permissions{
				Permissions: &collaboration.SharePermissions{
					Permissions: permissions.OcsPermissions(newPermissions).AsCS3Permissions(),
				},
			},
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedPerms := int(permissions.OCSFromCS3Permission(updatedShare.Permissions.Permissions))
	if retrievedPerms != newPermissions {
		t.Errorf("Expected share permissions to be updated, but they were not: got %d instead of %d", retrievedPerms, newPermissions)
	}
}

func TestUpdateReceivedShare(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	share, err := mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	receivedShare, err := mgr.GetReceivedShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Update the received share state
	receivedShare.State = collaboration.ShareState_SHARE_STATE_REJECTED

	_, err = mgr.UpdateReceivedShare(userctx, receivedShare, &field_mask.FieldMask{
		Paths: []string{"state"},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	updatedReceivedShare, err := mgr.GetReceivedShare(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if updatedReceivedShare.State != collaboration.ShareState_SHARE_STATE_REJECTED {
		t.Errorf("Expected received share state to be updated, but it was not")
	}
}

func TestShareWithGroup(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getGroupShareGrant("myegroup", "file")

	_, err = mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListShares(userctx, nil)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
	}

	if shares[0].Grantee.Type != provider.GranteeType_GRANTEE_TYPE_GROUP {
		t.Errorf("Expected share to be with a group, but it was not")
	}
}

func TestListSharesWithGranteeTypeFilter(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getGroupShareGrant("myegroup", "file")

	_, err = mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	filters := []*collaboration.Filter{
		{
			Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
			Term: &collaboration.Filter_GranteeType{
				GranteeType: provider.GranteeType_GRANTEE_TYPE_GROUP,
			},
		},
	}

	shares, err := mgr.ListShares(userctx, filters)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
	}
}

func TestListSharesWithMultipleFilters(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	sharegrant := getUserShareGrant("1000", "file")

	_, err = mgr.Share(userctx, file, sharegrant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	filters := []*collaboration.Filter{
		{
			Type: collaboration.Filter_TYPE_RESOURCE_ID,
			Term: &collaboration.Filter_ResourceId{ResourceId: &provider.ResourceId{
				StorageId: "project-b",
				OpaqueId:  "45468401564",
			}},
		},
		{
			Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
			Term: &collaboration.Filter_GranteeType{
				GranteeType: provider.GranteeType_GRANTEE_TYPE_USER,
			},
		},
	}

	shares, err := mgr.ListShares(userctx, filters)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 share, got %d", len(shares))
	}
}

func TestListExpiredShares(t *testing.T) {
	m, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}
	mgr := m.(*ShareMgr)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)

	expired := getUserShareGrant("1000", "file")
	expired.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
	future := getUserShareGrant("1001", "file")
	future.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}
	never := getUserShareGrant("1002", "file")

	var want string
	for _, g := range []*collaboration.ShareGrant{expired, future, never} {
		s, err := mgr.Share(userctx, file, g)
		if err != nil {
			t.Fatal(err)
		}
		if g == expired {
			want = s.Id.OpaqueId
		}
	}

	shares, err := mgr.ListExpiredShares(userctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].Id.OpaqueId != want {
		t.Fatalf("expected only share %s, got %v", want, shares)
	}
}

func TestMarkAsExpiredKeepsShare(t *testing.T) {
	m, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}
	mgr := m.(*ShareMgr)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grant := getUserShareGrant("1000", "file")
	grant.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
	s, err := mgr.Share(userctx, getRandomFile(user), grant)
	if err != nil {
		t.Fatal(err)
	}
	ref := &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: s.Id}}

	if err := mgr.MarkAsExpired(userctx, ref); err != nil {
		t.Fatal(err)
	}

	shares, err := mgr.ListExpiredShares(userctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 0 {
		t.Fatalf("expected no shares to treat, got %v", shares)
	}
	if _, err := mgr.GetShare(userctx, ref); err != nil {
		t.Fatalf("the owner no longer sees the share: %v", err)
	}
}

func TestUpdateExpirationOfTreatedShare(t *testing.T) {
	past := &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-time.Hour).Unix())}
	future := &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())}

	tests := []struct {
		name       string
		expiration *typesv1beta1.Timestamp
		// wantListed tells if the expired job must treat the share again once it expires.
		wantListed bool
	}{
		{name: "expiration removed", expiration: &typesv1beta1.Timestamp{}, wantListed: true},
		{name: "expiration moved to the future", expiration: future, wantListed: true},
		{name: "expiration moved within the past", expiration: past, wantListed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err, teardown := setupSuiteShares(t)
			defer teardown(t)
			if err != nil {
				t.Fatal(err)
			}
			mgr := m.(*ShareMgr)

			userctx := getUserContext("123456")
			user, _ := appctx.ContextGetUser(userctx)
			grant := getUserShareGrant("1000", "file")
			grant.Expiration = &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(-2 * time.Hour).Unix())}
			s, err := mgr.Share(userctx, getRandomFile(user), grant)
			if err != nil {
				t.Fatal(err)
			}
			ref := &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: s.Id}}
			if err := mgr.MarkAsExpired(userctx, ref); err != nil {
				t.Fatal(err)
			}

			updateExpiration := func(exp *typesv1beta1.Timestamp) *collaboration.Share {
				updated, err := mgr.UpdateShare(userctx, ref, &collaboration.UpdateShareRequest{
					Field: &collaboration.UpdateShareRequest_UpdateField{
						Field: &collaboration.UpdateShareRequest_UpdateField_Expiration{Expiration: exp},
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				return updated
			}

			updated := updateExpiration(tt.expiration)
			if updated.Expiration.GetSeconds() != tt.expiration.GetSeconds() {
				t.Fatalf("expected expiration %d, got %d", tt.expiration.GetSeconds(), updated.Expiration.GetSeconds())
			}

			// The share expires again: only a share whose treated mark was reset must show up.
			if tt.expiration != past {
				updateExpiration(past)
			}
			shares, err := mgr.ListExpiredShares(userctx)
			if err != nil {
				t.Fatal(err)
			}
			if listed := len(shares) == 1 && shares[0].Id.OpaqueId == s.Id.OpaqueId; listed != tt.wantListed || len(shares) > 1 {
				t.Fatalf("expected listed=%v, got %v", tt.wantListed, shares)
			}
		})
	}
}

// TestListShareSpaces asserts that every space holding a share comes back once,
// and that a space whose shares are all orphaned does not come back at all.
// This is what the reconciliation jobs walk, so it has to match what
// ListShares would return.
func TestListShareSpaces(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)

	// two shares in one space and one in another, so a space that holds more
	// than one share still comes back once.
	var inSpaceB *collaboration.Share
	for _, s := range []struct{ space, inode, sharee string }{
		{"space-a", "100", "1000"},
		{"space-a", "101", "1001"},
		{"space-b", "102", "1000"},
	} {
		file := getRandomFile(user)
		file.Id.SpaceId = s.space
		file.Id.OpaqueId = s.inode

		share, err := mgr.Share(userctx, file, getUserShareGrant(s.sharee, "file"))
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		inSpaceB = share
	}

	spaces, err := mgr.(*ShareMgr).ListShareSpaces(userctx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	sort.Strings(spaces)
	if len(spaces) != 2 || spaces[0] != "space-a" || spaces[1] != "space-b" {
		t.Errorf("Expected [space-a space-b], got %v", spaces)
	}

	err = mgr.(*ShareMgr).MarkAsOrphaned(userctx, &collaboration.ShareReference{
		Spec: &collaboration.ShareReference_Id{Id: inSpaceB.Id},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	spaces, err = mgr.(*ShareMgr).ListShareSpaces(userctx)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	if len(spaces) != 1 || spaces[0] != "space-a" {
		t.Errorf("Expected [space-a], got %v", spaces)
	}
}

func shareRefByID(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: id}}}
}

// A legacy row can have an initiator that is not the owner: both of them see
// the share, and any other user gets not found, never permission denied.
func TestGetShareOfOwnerOrInitiator(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}

	ownerctx := getUserContext("owner")
	owner, _ := appctx.ContextGetUser(ownerctx)
	share, err := mgr.Share(ownerctx, getRandomFile(owner), getUserShareGrant("sharee", "file"))
	if err != nil {
		t.Fatal(err)
	}
	ref := shareRefByID(share.Id.OpaqueId)
	if err := mgr.(*ShareMgr).TransferShare(ownerctx, ref, "initiator"); err != nil {
		t.Fatal(err)
	}

	for _, u := range []string{"owner", "initiator"} {
		got, err := mgr.GetShare(getUserContext(u), ref)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		if got.Owner.OpaqueId != "owner" || got.Creator.OpaqueId != "initiator" {
			t.Fatalf("%s: got owner %s and creator %s", u, got.Owner.OpaqueId, got.Creator.OpaqueId)
		}
	}

	// No gateway is configured, so the path of the share cannot be resolved:
	// that also answers not found.
	_, err = mgr.GetShare(getUserContext("other"), ref)
	if _, ok := err.(errtypes.IsNotFound); !ok {
		t.Fatalf("another user: got %v, want not found", err)
	}
}

func TestListSharesByOwner(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, owner := range []string{"alice", "bob"} {
		userctx := getUserContext(owner)
		user, _ := appctx.ContextGetUser(userctx)
		file := getRandomFile(user)
		file.Id.OpaqueId = owner
		if _, err := mgr.Share(userctx, file, getUserShareGrant("sharee", "file")); err != nil {
			t.Fatal(err)
		}
	}

	ownerFilter := func(u string) *collaboration.Filter {
		return &collaboration.Filter{Type: collaboration.Filter_TYPE_OWNER, Term: &collaboration.Filter_Owner{Owner: &userpb.UserId{OpaqueId: u}}}
	}
	shares, err := mgr.(*ShareMgr).ListModelShares(nil, []*collaboration.Filter{ownerFilter("alice")}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 1 || shares[0].UIDOwner != "alice" {
		t.Fatalf("got %v, want the share of alice", shares)
	}
	shares, err = mgr.(*ShareMgr).ListModelShares(nil, []*collaboration.Filter{ownerFilter("alice"), ownerFilter("bob")}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 2 {
		t.Fatalf("got %d shares, want 2", len(shares))
	}
}

func TestMoveShareToAnotherSpace(t *testing.T) {
	mgr, err, teardown := setupSuiteShares(t)
	defer teardown(t)
	if err != nil {
		t.Fatal(err)
	}

	userctx := getUserContext("owner")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	file.Id.SpaceId = "space-a"
	share, err := mgr.Share(userctx, file, getUserShareGrant("sharee", "file"))
	if err != nil {
		t.Fatal(err)
	}
	ref := shareRefByID(share.Id.OpaqueId)
	if err := mgr.(*ShareMgr).MoveShare(userctx, ref, &provider.ResourceId{StorageId: "store-b", OpaqueId: "200", SpaceId: "space-b"}, "newowner"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.(*ShareMgr).GetShareUnfiltered(userctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.UIDOwner != "newowner" || got.Instance != "store-b" || got.Inode != "200" || got.SpaceID != "space-b" {
		t.Fatalf("got %+v", got.ProtoShare)
	}
}
