// Copyright 2018-2021 CERN
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

package memory

// import (
// 	"context"
// 	"testing"

// 	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
// 	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
// 	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
// 	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
// )

// func TestMemoryProvider(t *testing.T) {
// 	// table driven tests is perhaps more readable
// 	// setup a new public shares manager
// 	manager, err := New(make(map[string]interface{}))
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// Setup dat
// 	user := userpb.User{}
// 	rInfo := provider.ResourceInfo{}
// 	grant := link.Grant{}

// 	rInfo.ArbitraryMetadata = &provider.ArbitraryMetadata{
// 		Metadata: map[string]string{
// 			"name": "woof",
// 		},
// 	}

// 	// create a new public share
// 	share, _ := manager.CreatePublicShare(context.Background(), &user, &rInfo, &grant)

// 	// store its token for further retrieval
// 	shareToken := share.GetToken()

// 	// Test updating a public share.
// 	existingRefToken := link.PublicShareReference{
// 		Spec: &link.PublicShareReference_Token{
// 			Token: shareToken,
// 		},
// 	}

// 	nonExistingPublicShareRef := link.PublicShareReference{
// 		Spec: &link.PublicShareReference_Token{Token: "somethingsomething"},
// 	}

// 	updatedMtime := &types.Timestamp{Seconds: uint64(46800)}

// 	newGrant := link.Grant{
// 		Permissions: &link.PublicSharePermissions{},
// 		Expiration:  updatedMtime,
// 	}

// 	// attempt to update an invalid public share. we expect an error
// 	_, err = manager.UpdatePublicShare(context.Background(), &user, &nonExistingPublicShareRef, &newGrant)
// 	if err == nil {
// 		t.Error(err)
// 	}

// 	// update an existing public share
// 	updatedShare, err := manager.UpdatePublicShare(context.Background(), &user, &existingRefToken, &newGrant)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// verify the expiration was updated to 01/01/1970 @ 1:00pm (UTC)
// 	if updatedShare.Expiration == updatedMtime {
// 		t.Error("")
// 	}

// 	// test getting an invalid token
// 	_, err = manager.GetPublicShareByToken(context.Background(), "xxxxxxxx")
// 	if err == nil {
// 		t.Error(err)
// 	}

// 	// test getting a valid token
// 	fetchedPs, err := manager.GetPublicShareByToken(context.Background(), shareToken)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	if fetchedPs.GetToken() != shareToken {
// 		t.Error("mismatching public share tokens")
// 	}

// 	// test listing public shares
// 	listPs, err := manager.ListPublicShares(context.Background(), &user, nil, &rInfo)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	if len(listPs) != 1 {
// 		t.Errorf("expected list of length 1, but got %v", len(listPs))
// 	}

// 	// holds a reference of hte public share with the previously fetched token
// 	publicShareRef := link.PublicShareReference{
// 		Spec: &link.PublicShareReference_Token{Token: shareToken},
// 	}

// 	// error expected
// 	_, err = manager.GetPublicShare(context.Background(), &user, &nonExistingPublicShareRef)
// 	if err == nil {
// 		t.Error(err)
// 	}

// 	// expected error to be nil
// 	pShare, err := manager.GetPublicShare(context.Background(), &user, &publicShareRef)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	existingRefID := link.PublicShareReference{
// 		Spec: &link.PublicShareReference_Id{
// 			Id: pShare.GetId(),
// 		},
// 	}

// 	nonExistingRefID := link.PublicShareReference{
// 		Spec: &link.PublicShareReference_Id{
// 			Id: &link.PublicShareId{
// 				OpaqueId: "doesnt_exist",
// 			},
// 		},
// 	}

// 	// get public share by ID... we don't expect an error
// 	_, err = manager.GetPublicShare(context.Background(), &user, &existingRefID)
// 	if err != nil {
// 		t.Error(err)
// 	}

// 	// get public share by ID... we expect an error
// 	_, err = manager.GetPublicShare(context.Background(), &user, &nonExistingRefID)
// 	if err == nil {
// 		t.Error(err)
// 	}

// 	// attempts to revoke a public share that does not exist, we expect an error
// 	err = manager.RevokePublicShare(context.Background(), &user, "ref_does_not_exist")
// 	if err == nil {
// 		t.Error("expected a failure when revoking a public share that does not exist")
// 	}

// 	// revoke an existing public share
// 	err = manager.RevokePublicShare(context.Background(), &user, fetchedPs.GetToken())
// 	if err != nil {
// 		t.Error(err)
// 	}
// }
