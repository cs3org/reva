package memory

import (
	"context"
	"testing"

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	shareProviderpb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	types "github.com/cs3org/go-cs3apis/cs3/types"
)

func TestMemoryProvider(t *testing.T) {
	// TODO: table driven tests is perhaps more readable
	// setup a new public shares manager
	manager, err := New()
	if err != nil {
		t.Error(err)
	}

	// Setup dat
	user := authv0alphapb.User{}
	rInfo := storageproviderv0alphapb.ResourceInfo{}
	grant := shareProviderpb.Grant{}

	// create a new public share
	share, _ := manager.CreatePublicShare(context.Background(), &user, &rInfo, &grant)

	// store its token for further retrieval
	shareToken := share.GetToken()

	// Test updating a public share. test with --race
	existingRefToken := shareProviderpb.PublicShareReference{
		Spec: &shareProviderpb.PublicShareReference_Token{
			Token: shareToken,
		},
	}

	nonExistingPublicShareRef := shareProviderpb.PublicShareReference{
		Spec: &shareProviderpb.PublicShareReference_Token{Token: "somethingsomething"},
	}

	updatedMtime := &types.Timestamp{Seconds: uint64(46800)}

	newGrant := shareProviderpb.Grant{
		Permissions: &shareProviderpb.PublicSharePermissions{
			Permissions: &storageproviderv0alphapb.ResourcePermissions{}, // add some permissions maybe?
		},
		Expiration: updatedMtime,
	}

	// attempt to update an invalid public share. we expect an error
	_, err = manager.UpdatePublicShare(context.Background(), &user, &nonExistingPublicShareRef, &newGrant)
	if err == nil {
		t.Error(err)
	}

	// update an existing public share
	updatedShare, err := manager.UpdatePublicShare(context.Background(), &user, &existingRefToken, &newGrant)
	if err != nil {
		t.Error(err)
	}

	// verify the expiration was updated to 01/01/1970 @ 1:00pm (UTC)
	if updatedShare.Expiration == updatedMtime {
		t.Error("")
	}

	// test getting an invalid token
	_, err = manager.GetPublicShareByToken(context.Background(), "xxxxxxxx")
	if err == nil {
		t.Error(err)
	}

	// test getting a valid token
	fetchedPs, err := manager.GetPublicShareByToken(context.Background(), shareToken)
	if err != nil {
		t.Error(err)
	}

	if fetchedPs.GetToken() != shareToken {
		t.Error("mismatching public share tokens")
	}

	// test listing public shares
	listPs, err := manager.ListPublicShares(context.Background(), &user, &rInfo)
	if err != nil {
		t.Error(err)
	}

	if len(listPs) != 1 {
		t.Errorf("expected list of length 1, but got %v", len(listPs))
	}

	// holds a reference of hte public share with the previously fetched token
	publicShareRef := shareProviderpb.PublicShareReference{
		Spec: &shareProviderpb.PublicShareReference_Token{Token: shareToken},
	}

	// error expected
	_, err = manager.GetPublicShare(context.Background(), &user, &nonExistingPublicShareRef)
	if err == nil {
		t.Error(err)
	}

	// expected error to be nil
	pShare, err := manager.GetPublicShare(context.Background(), &user, &publicShareRef)
	if err != nil {
		t.Error(err)
	}

	existingRefID := shareProviderpb.PublicShareReference{
		Spec: &shareProviderpb.PublicShareReference_Id{
			Id: pShare.GetId(),
		},
	}

	nonExistingRefID := shareProviderpb.PublicShareReference{
		Spec: &shareProviderpb.PublicShareReference_Id{
			Id: &shareProviderpb.PublicShareId{
				OpaqueId: "doesnt_exist",
			},
		},
	}

	// get public share by ID... we don't expect an error
	_, err = manager.GetPublicShare(context.Background(), &user, &existingRefID)
	if err != nil {
		t.Error(err)
	}

	// get public share by ID... we expect an error
	_, err = manager.GetPublicShare(context.Background(), &user, &nonExistingRefID)
	if err == nil {
		t.Error(err)
	}

	// attemts to revoke a public share that does not exist, we expect an error
	err = manager.RevokePublicShare(context.Background(), &user, "ref_does_not_exist")
	if err == nil {
		t.Error("expected a failure when revoking a public share that does not exist")
	}

	// revoke an existing public share
	err = manager.RevokePublicShare(context.Background(), &user, fetchedPs.GetToken())
	if err != nil {
		t.Error(err)
	}
}
