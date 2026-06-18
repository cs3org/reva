package sql

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	//permissions "github.com/cs3org/reva/v3/pkg/cbox/utils"
	"github.com/cs3org/reva/v3/pkg/permissions"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/genproto/protobuf/field_mask"
)

// ===========================
// Helper functions for tests
// ===========================

// You can use testing.T, if you want to test the code without benchmarking
func setupSuiteOcmShares(tb testing.TB) (share.Repository, error, func(tb testing.TB)) {
	ctx := context.Background()
	dbName := "test_db.sqlite"
	cfg := map[string]any{
		"db_engine": "sqlite",
		"db_name":   dbName,
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
		return share.NewWebDAVProtocol(uri, sharedsecret, perms, []ocm.AccessType{}, []string{})
	case "editor":
		return share.NewWebDAVProtocol(uri, sharedsecret, perms, []ocm.AccessType{}, []string{})
	}
	return nil
}

func getWebAppProtocol(appURL string, role string) *ocm.Protocol {
	var perms *provider.ResourcePermissions
	switch role {
	case "viewer":
		perms = permissions.NewViewerRole().CS3ResourcePermissions()
	case "editor":
		perms = permissions.NewEditorRole().CS3ResourcePermissions()
	default:
		return nil
	}
	return share.NewWebappProtocol(appURL, "sharedsecret", perms,
		share.DefaultWebappRequirements, share.DefaultWebappTargets, "", "", nil)
}

func getProtocols(ocsPermissions permissions.OcsPermissions, resource_type string, role string) []*ocm.Protocol {
	perms := &ocm.SharePermissions{
		Permissions: permissions.RoleFromOCSPermissions(ocsPermissions).CS3ResourcePermissions(), //conversions.IntTosharePerm(permissions, resource_type),
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
			share.NewWebDavAccessMethod(permissions.NewViewerRole().CS3ResourcePermissions(), []ocm.AccessType{}, []string{}),
			share.NewWebappAccessMethod(
				permissions.NewViewerRole().CS3ResourcePermissions(),
				share.DefaultWebappRequirements, ""),
		}
	case "editor":
		return []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), []ocm.AccessType{}, []string{}),
			share.NewWebappAccessMethod(
				permissions.NewEditorRole().CS3ResourcePermissions(),
				share.DefaultWebappRequirements, ""),
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

func getOcmAccessMethodsWithCodeFlow(role string) []*ocm.AccessMethod {
	reqs := []string{"must-exchange-token"}
	ats := []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE}
	switch role {
	case "viewer":
		return []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(permissions.NewViewerRole().CS3ResourcePermissions(), ats, reqs),
			share.NewWebappAccessMethod(
				permissions.NewViewerRole().CS3ResourcePermissions(),
				share.DefaultWebappRequirements, ""),
		}
	case "editor":
		return []*ocm.AccessMethod{
			share.NewWebDavAccessMethod(permissions.NewEditorRole().CS3ResourcePermissions(), ats, reqs),
			share.NewWebappAccessMethod(
				permissions.NewEditorRole().CS3ResourcePermissions(),
				share.DefaultWebappRequirements, ""),
		}
	}
	return []*ocm.AccessMethod{}
}

func getOCMReceivedShareWithCodeFlow(user *userpb.User, grantee *provider.Grantee, receivedShareID string) *ocm.ReceivedShare {
	perms := &ocm.SharePermissions{
		Permissions: permissions.NewViewerRole().CS3ResourcePermissions(),
	}
	reqs := []string{"must-exchange-token"}
	ats := []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE}

	return &ocm.ReceivedShare{
		Name:          "receivedshare",
		RemoteShareId: receivedShareID,
		Grantee:       grantee,
		Owner:         user.Id,
		Creator:       user.Id,
		Ctime:         &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Unix())},
		Mtime:         &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Unix())},
		Expiration:    &typesv1beta1.Timestamp{Seconds: uint64(time.Now().Add(24 * time.Hour).Unix())},
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		ResourceType:  provider.ResourceType_RESOURCE_TYPE_FILE,
		Protocols: []*ocm.Protocol{
			share.NewWebDAVProtocol("https://webdav.example.com/remote.php/dav/shares/someid", "sharedsecret", perms, ats, reqs),
			share.NewWebappProtocol("https://webapp.example.com/apps/files/?dir=/Shares/someid", "sharedsecret",
				permissions.NewViewerRole().CS3ResourcePermissions(),
				share.DefaultWebappRequirements, share.DefaultWebappTargets, "", "", nil),
		},
	}
}

func findWebDAVAccessMethod(methods []*ocm.AccessMethod) *ocm.WebDAVAccessMethod {
	for _, m := range methods {
		if wdav, ok := m.Term.(*ocm.AccessMethod_WebdavOptions); ok {
			return wdav.WebdavOptions
		}
	}
	return nil
}

func findWebDAVProtocol(protocols []*ocm.Protocol) *ocm.WebDAVProtocol {
	for _, p := range protocols {
		if wdav, ok := p.Term.(*ocm.Protocol_WebdavOptions); ok {
			return wdav.WebdavOptions
		}
	}
	return nil
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

func TestGetOCMShareByIdAllowsFederatedGrantee(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	ownerCtx := getUserContext("owner1")
	owner, _ := appctx.ContextGetUser(ownerCtx)
	owner.Id.Idp = "cernbox1.docker"

	grantee := getUserOcmShareGrantee("michiel")
	grantee.GetUserId().Idp = "nextcloud1.docker"
	accessMethods := getOcmAccessMethods("viewer")

	share, err := mgr.StoreShare(ownerCtx, getOcmShare(accessMethods, grantee, owner.Id, getResourceId(), "federatedtoken"))
	if err != nil {
		t.Fatal(err)
	}

	recipientCtx := getUserContext("michiel")
	recipient, _ := appctx.ContextGetUser(recipientCtx)
	recipient.Id.Idp = "nextcloud1.docker"

	ref := ocm.ShareReference{
		Spec: &ocm.ShareReference_Id{
			Id: share.Id,
		},
	}

	retrievedShare, err := mgr.GetShare(recipientCtx, recipient, &ref)
	if err != nil {
		t.Fatal(err)
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

	shares, err := mgr.ListReceivedShares(userctx, user, []*ocm.ListReceivedOCMSharesRequest_Filter{})
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

func TestOutgoingShareRoundTripsRequirementsAndAccessTypes(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethodsWithCodeFlow("viewer")

	stored, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "codeflowtoken"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}
	retrieved, err := mgr.GetShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAVAccessMethod(retrieved.AccessMethods)
	if wdav == nil {
		t.Fatal("expected WebDAV access method on retrieved share")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements: got %v, want [must-exchange-token]", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types: got %v, want [ACCESS_TYPE_REMOTE]", wdav.AccessTypes)
	}
}

func TestReceivedShareRoundTripsRequirementsAndAccessTypes(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("sharee1")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")

	receivedShare := getOCMReceivedShareWithCodeFlow(user, grantee, "remote-cf-1")
	stored, err := mgr.StoreReceivedShare(userctx, receivedShare)
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}
	retrieved, err := mgr.GetReceivedShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAVProtocol(retrieved.Protocols)
	if wdav == nil {
		t.Fatal("expected WebDAV protocol on retrieved received share")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements: got %v, want [must-exchange-token]", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types: got %v, want [ACCESS_TYPE_REMOTE]", wdav.AccessTypes)
	}
}

func TestUpdatePreservesRequirementsWhenFieldsOmitted(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethodsWithCodeFlow("viewer")

	stored, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "cftoken-ocs"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	// OCS caller shape: sends Permissions only, omits Requirements and AccessTypes
	_, err = mgr.UpdateShare(userctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
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

	retrieved, err := mgr.GetShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAVAccessMethod(retrieved.AccessMethods)
	if wdav == nil {
		t.Fatal("expected WebDAV access method after update")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements lost after OCS-shape update: got %v", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types lost after OCS-shape update: got %v", wdav.AccessTypes)
	}
}

func TestUpdatePreservesRequirementsWithExplicitEmptySlice(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethodsWithCodeFlow("viewer")

	stored, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "cftoken-graph"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	// ocGraph/CLI caller shape: sends explicit Requirements: []string{}
	_, err = mgr.UpdateShare(userctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
		Field: &ocm.UpdateOCMShareRequest_UpdateField_AccessMethods{
			AccessMethods: &ocm.AccessMethod{
				Term: &ocm.AccessMethod_WebdavOptions{
					WebdavOptions: &ocm.WebDAVAccessMethod{
						Permissions:  permissions.NewEditorRole().CS3ResourcePermissions(),
						Requirements: []string{},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	retrieved, err := mgr.GetShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}

	wdav := findWebDAVAccessMethod(retrieved.AccessMethods)
	if wdav == nil {
		t.Fatal("expected WebDAV access method after update")
	}
	if len(wdav.Requirements) != 1 || wdav.Requirements[0] != "must-exchange-token" {
		t.Errorf("requirements lost after ocGraph-shape update: got %v", wdav.Requirements)
	}
	if len(wdav.AccessTypes) != 1 || wdav.AccessTypes[0] != ocm.AccessType_ACCESS_TYPE_REMOTE {
		t.Errorf("access_types lost after ocGraph-shape update: got %v", wdav.AccessTypes)
	}
}

func TestUpdateRejectsDifferentRequirements(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("123456")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")
	accessMethods := getOcmAccessMethodsWithCodeFlow("viewer")

	stored, err := mgr.StoreShare(userctx, getOcmShare(accessMethods, grantee, user.Id, getResourceId(), "cftoken-reject"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: stored.Id}}

	_, err = mgr.UpdateShare(userctx, user, ref, &ocm.UpdateOCMShareRequest_UpdateField{
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
		t.Fatal("expected error when updating with different requirements, got nil")
	}
}

func TestUpdateReceivedOCMShareHidden(t *testing.T) {
	mgr, err, teardown := setupSuiteOcmShares(t)
	defer teardown(t)

	userctx := getUserContext("sharee1")
	user, _ := appctx.ContextGetUser(userctx)
	grantee := getUserOcmShareGrantee("sharee1")

	stored, err := mgr.StoreReceivedShare(userctx, getOCMReceivedShare(user, grantee, "file", "viewer", "remoteshareid-hidden"))
	if err != nil {
		t.Fatal(err)
	}

	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: stored.Id.OpaqueId}}}

	// Newly stored share must not be hidden.
	got, err := mgr.GetReceivedShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetHidden() {
		t.Fatal("expected freshly stored share to not be hidden")
	}

	// Hide the share via the dedicated hidden flag.
	toUpdate := &ocm.ReceivedShare{Id: &ocm.ShareId{OpaqueId: stored.Id.OpaqueId}, Hidden: true}
	if _, err := mgr.UpdateReceivedShare(userctx, user, toUpdate, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil {
		t.Fatal(err)
	}

	// Re-read from the DB and verify the hidden flag was persisted.
	got, err = mgr.GetReceivedShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetHidden() {
		t.Fatal("expected share to be hidden after update")
	}

	// Unhide and verify it round-trips back.
	toUpdate = &ocm.ReceivedShare{Id: &ocm.ShareId{OpaqueId: stored.Id.OpaqueId}, Hidden: false}
	if _, err := mgr.UpdateReceivedShare(userctx, user, toUpdate, &field_mask.FieldMask{Paths: []string{"hidden"}}); err != nil {
		t.Fatal(err)
	}
	got, err = mgr.GetReceivedShare(userctx, user, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetHidden() {
		t.Fatal("expected share to not be hidden after unhide")
	}
}
