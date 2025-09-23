package sql

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"
	publicshare "github.com/cs3org/reva/v3/pkg/publicshare"
)

func setupSuiteLinks(tb testing.TB) (publicshare.Manager, error, func(tb testing.TB)) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]interface{}{
		"engine":  "sqlite",
		"db_name": dbName,
	}
	mgr, err := NewPublicShareManager(ctx, cfg)
	if err != nil {
		os.Remove(dbName)
		return nil, err, nil
	}

	// Return a function to teardown the test
	return mgr, nil, func(tb testing.TB) {
		log.Println("teardown suite")
		os.Remove(dbName)
	}
}

func getTestPublicLinkGrant(password string) *link.Grant {
	return &link.Grant{
		Permissions: &link.PublicSharePermissions{
			Permissions: conversions.IntTosharePerm(1, "file"),
		},
		Password: password,
		Expiration: &typespb.Timestamp{
			Seconds: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
	}
}

func TestCreatePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	publicShare, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if publicShare.Token == "" {
		t.Error("Expected public share token to be generated")
	}

	if publicShare.Owner.OpaqueId != user.Id.OpaqueId {
		t.Errorf("Expected owner ID %s, got %s", user.Id.OpaqueId, publicShare.Owner.OpaqueId)
	}
}

func TestCreatePublicShareWithPassword(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	passwordGrant := getTestPublicLinkGrant("secret")

	passwordPublicShare, err := mgr.CreatePublicShare(userctx, nil, file, passwordGrant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	unprotectedPublicShare, err := mgr.CreatePublicShare(userctx, nil, file, getTestPublicLinkGrant(""), "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if !passwordPublicShare.PasswordProtected {
		t.Error("Expected share to be password protected")
	}

	if unprotectedPublicShare.PasswordProtected {
		t.Error("Did not expect unprotected share to be password protected")
	}

	if passwordPublicShare.Token == "" || unprotectedPublicShare.Token == "" {
		t.Error("Expected a token to be generated")
	}
}

func TestGetPublicShareByToken(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	createdFirst, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	createdSecond, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedFirst, err := mgr.GetPublicShareByToken(userctx, createdFirst.Token, nil, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedFirst.Token != createdFirst.Token {
		t.Errorf("Expected token %s, got %s", createdFirst.Token, retrievedFirst.Token)
	}

	retrievedSecond, err := mgr.GetPublicShareByToken(userctx, createdSecond.Token, nil, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedSecond.Token != createdSecond.Token {
		t.Errorf("Expected token %s, got %s", createdSecond.Token, retrievedSecond.Token)
	}
}

func TestUpdatePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Test updating display name
	newName := "Updated Name"
	updateReq := &link.UpdatePublicShareRequest{
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Id{
				Id: share.Id,
			},
		},
		Update: &link.UpdatePublicShareRequest_Update{
			Type:        link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME,
			DisplayName: newName,
		},
	}

	updated, err := mgr.UpdatePublicShare(userctx, nil, updateReq, grant)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if updated.DisplayName != newName {
		t.Errorf("Expected display name %s, got %s", newName, updated.DisplayName)
	}
}

func TestRevokePublicShare(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	err = mgr.RevokePublicShare(userctx, nil, &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{
			Id: share.Id,
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// Verify share is deleted
	_, err = mgr.GetPublicShare(userctx, nil, &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{
			Id: share.Id,
		},
	}, false)
	if err == nil {
		t.Error("Expected share to be deleted")
	}
}

func TestListPublicShares(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	_, err = mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListPublicShares(userctx, nil, nil, file, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 public share, got %d", len(shares))
	}
}

func TestListPublicSharesWithFilters(t *testing.T) {
	mgr, err, teardown := setupSuiteLinks(t)
	defer teardown(t)

	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	file := getRandomFile(user)
	grant := getTestPublicLinkGrant("")

	share, err := mgr.CreatePublicShare(userctx, nil, file, grant, "test description", false, false, "")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	filters := []*link.ListPublicSharesRequest_Filter{
		{
			Type: link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
			Term: &link.ListPublicSharesRequest_Filter_ResourceId{
				ResourceId: &provider.ResourceId{
					StorageId: file.Id.StorageId,
					OpaqueId:  file.Id.OpaqueId,
				},
			},
		},
	}

	shares, err := mgr.ListPublicShares(userctx, nil, filters, file, false)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 1 public share, got %d", len(shares))
	}

	if shares[0].Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected share ID %s, got %s", share.Id.OpaqueId, shares[0].Id.OpaqueId)
	}
}
