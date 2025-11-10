package sql

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocsconversions "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	conversions "github.com/cs3org/reva/v3/pkg/cbox/utils"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
)

// ===========================
// Helper functions for tests
// ===========================

// You can use testing.T, if you want to test the code without benchmarking
func setupSuiteOcmShares(tb testing.TB) (share.Repository, error, func(tb testing.TB)) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]interface{}{
		"engine":  "sqlite",
		"db_name": dbName,
	}
	mgr, err := NewOCMShareManager(ctx, cfg)
	if err != nil {
		return nil, err, nil
	}

	// Return a function to teardown the test
	return mgr, nil, func(tb testing.TB) {
		log.Println("teardown suite")
		os.Remove(dbName)
	}
}

// Add resourceId parameter
func getOcmShare(accessMethods []*ocm.AccessMethod, grantee *provider.Grantee, creator *userpb.UserId, resourceId *provider.ResourceId, token string) *ocm.Share {

	return &ocm.Share{
		ResourceId: resourceId,
		Name:       "testshare",
		Token:      token,
		Grantee:    grantee,
		Owner:      creator,
		Creator:    creator,
		Ctime: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Unix()),
		},
		Mtime: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Unix()),
		},
		Expiration: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		AccessMethods: accessMethods,
	}
}

func getWebDavProtocol(uri string, sharedsecret string, perms *ocm.SharePermissions, role string) *ocm.Protocol {
	switch role {
	case "viewer":
		return share.NewWebDAVProtocol(uri, sharedsecret, perms, []string{})
	case "editor":
		return share.NewWebDAVProtocol(uri, sharedsecret, perms, []string{})
	}
	return nil
}

func getWebAppProtocol(appURL string, role string) *ocm.Protocol {
	switch role {
	case "viewer":
		return share.NewWebappProtocol(appURL, appprovider.ViewMode_VIEW_MODE_READ_ONLY)
	case "editor":
		return share.NewWebappProtocol(appURL, appprovider.ViewMode_VIEW_MODE_READ_WRITE)
	}
	return nil
}

func getProtocols(permissions int, resource_type string, role string) []*ocm.Protocol {
	perms := &ocm.SharePermissions{
		Permissions: conversions.IntTosharePerm(permissions, resource_type),
	}
	protocols := []*ocm.Protocol{
		getWebDavProtocol("https://webdav.example.com/remote.php/dav/shares/someid", "sharedsecret", perms, role),
		getWebAppProtocol("https://webapp.example.com/apps/files/?dir=/Shares/someid", role),
	}
	return protocols
}

func getOCMReceivedShare(user *userpb.User, grantee *provider.Grantee, resource_type string, role string, received_share_id string) *ocm.ReceivedShare {

	receivedShare := &ocm.ReceivedShare{

		Name:          "receivedshare",
		RemoteShareId: received_share_id,
		Grantee:       grantee,
		Owner:         user.Id,
		Creator:       user.Id,
		Ctime: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Unix()),
		},
		Mtime: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Unix()),
		},
		Expiration: &typesv1beta1.Timestamp{
			Seconds: uint64(time.Now().Add(24 * time.Hour).Unix()),
		},
		ShareType:    ocm.ShareType_SHARE_TYPE_USER,
		Protocols:    getProtocols(1, resource_type, role),
		ResourceType: provider.ResourceType_RESOURCE_TYPE_FILE,
	}
	return receivedShare
}

func getUserOcmShareGrantee(shareeId string) *provider.Grantee {
	sharee := &provider.Grantee{
		Type: provider.GranteeType_GRANTEE_TYPE_USER,
		Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{
				Type:     userpb.UserType_USER_TYPE_APPLICATION,
				OpaqueId: shareeId,
			},
		},
	}

	return sharee
}

func getOcmAccessMethods(role string) []*ocm.AccessMethod {
	switch role {
	case "viewer":
		return []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(ocsconversions.NewViewerRole().CS3ResourcePermissions(), []string{}),
			share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_ONLY),
		}
	case "editor":
		return []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(ocsconversions.NewEditorRole().CS3ResourcePermissions(), []string{}),
			share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
		}
	}
	return []*ocm.AccessMethod{}
}

func getResourceId() *provider.ResourceId {
	return &provider.ResourceId{
		StorageId: "storageid",
		OpaqueId:  "opaqueid",
	}
}

// ===========================
//        Actual tests
// ===========================

func TestGetOCMShareById(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	share, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Id{
			Id: share.Id,
		},
	}

	retrievedShare, err := mgr.GetShare(userctx, user, &ref)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedShare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected share ID %s, got %s", share.Id.OpaqueId, retrievedShare.Id.OpaqueId)
	}
}

func TestGetOcmShareByKey(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Key{
			Key: &ocm.ShareKey{
				Owner:      user.Id,
				ResourceId: getResourceId(),
				Grantee:    grantee,
			},
		},
	}

	share, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedShare, err := mgr.GetShare(userctx, user, &ref)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedShare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected share ID %s, got %s", share.Id.OpaqueId, retrievedShare.Id.OpaqueId)
	}
}

func TestGetOCMShareByToken(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Token{
			Token: "sometoken",
		},
	}

	share, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	retrievedShare, err := mgr.GetShare(userctx, user, &ref)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if retrievedShare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected share ID %s, got %s", share.Id.OpaqueId, retrievedShare.Id.OpaqueId)
	}
}

func TestDoNotCreateTheSameShareTwice(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err == nil {
		t.Error("Expected error when creating the same share twice, but got none")
		t.FailNow()
	}
}

func TestListOCMShares(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee1 := getUserOcmShareGrantee("sharee1")
	grantee2 := getUserOcmShareGrantee("sharee2")
	accessMethods := getOcmAccessMethods("viewer")

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee1, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee2, user.Id, getResourceId(), "someothertoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListShares(userctx, user, []*ocm.ListOCMSharesRequest_Filter{share.ResourceIDFilter(getResourceId())})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 2 {
		t.Errorf("Expected 2 shares, got %d", len(shares))
	}
}

func TestStoreReceivedOCMShare(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("sharee1")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")

	receivedShare := getOCMReceivedShare(user, grantee, "file", "viewer", "someremoteshareid")

	storedReceivedShare, err := mgr.StoreReceivedShare(userctx, receivedShare)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	ref := &ocm.ShareReference{
		Spec: &ocm.ShareReference_Id{
			Id: &ocm.ShareId{
				OpaqueId: storedReceivedShare.Id.OpaqueId,
			},
		},
	}

	share, err := mgr.GetReceivedShare(userctx, user, ref)

	if storedReceivedShare.Id.OpaqueId != share.Id.OpaqueId {
		t.Errorf("Expected received share ID %s, got %s", share.Id.OpaqueId, storedReceivedShare.Id.OpaqueId)
	}
}

func TestListOCMSharesReceived(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("sharee1")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")

	receivedShare1 := getOCMReceivedShare(user, grantee, "file", "viewer", "someremoteshareid")
	receivedShare2 := getOCMReceivedShare(user, grantee, "file", "editor", "someotherremoteshareid")

	_, err = mgr.StoreReceivedShare(userctx, receivedShare1)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.StoreReceivedShare(userctx, receivedShare2)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListReceivedShares(userctx, user)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 2 {
		t.Errorf("Expected 2 received shares, got %d", len(shares))
	}
}

func TestDeleteOCMShare(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	share, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Id{
			Id: share.Id,
		},
	}

	err = mgr.DeleteShare(userctx, user, &ref)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, err = mgr.GetShare(userctx, user, &ref)
	if err == nil {
		t.Error("Expected error when getting a deleted share, but got none")
		t.FailNow()
	}
}

func TestUpdateOCMShare(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethods("viewer")

	share, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Id{
			Id: share.Id,
		},
	}

	updatedShare, err := mgr.UpdateShare(userctx, user, &ref, &ocm.UpdateOCMShareRequest_UpdateField{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_Expiration{
			Expiration: &typesv1beta1.Timestamp{
				Seconds: uint64(time.Now().Add(48 * time.Hour).Unix()),
			},
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if updatedShare.Expiration.Seconds <= share.Expiration.Seconds {
		t.Error("Expected expiration to be updated to a later time")
		t.FailNow()
	}
}

func TestListOCMSharesWithOwnerFilter(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee1 := getUserOcmShareGrantee("sharee1")
	grantee2 := getUserOcmShareGrantee("sharee2")
	accessMethods := getOcmAccessMethods("viewer")

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee1, user.Id, getResourceId(), "sometoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	u := &userpb.UserId{
		OpaqueId: "somename",
		Type:     userpb.UserType_USER_TYPE_APPLICATION,
	}

	_, err = mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee2, u, getResourceId(), "someothertoken"))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	shares, err := mgr.ListShares(userctx, user, []*ocm.ListOCMSharesRequest_Filter{
		{
			Type: ocm.ListOCMSharesRequest_Filter_TYPE_OWNER,
			Term: &ocm.ListOCMSharesRequest_Filter_Owner{
				Owner: user.Id,
			},
		},
	})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(shares) != 1 {
		t.Errorf("Expected 2 shares, got %d", len(shares))
	}
}
