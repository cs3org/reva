package sql

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	groupv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"
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
	cfg := map[string]interface{}{
		"engine":  "sqlite",
		"db_name": dbName,
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
			Permissions: conversions.IntTosharePerm(1, resourcetype),
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
			Permissions: conversions.IntTosharePerm(1, resourcetype),
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
			Permissions: conversions.IntTosharePerm(1, "file"),
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
					Permissions: conversions.IntTosharePerm(newPermissions, "file"),
				},
			},
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedPerms := conversions.SharePermToInt(updatedShare.Permissions.Permissions)
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
