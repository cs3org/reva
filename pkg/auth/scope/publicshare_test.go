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

package scope

import (
	"context"
	"errors"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog/log"

	"gotest.tools/assert"
)

func getPublicShareScope(resourceID *providerv1beta1.ResourceId, publicLinkRole authpb.Role) (*authpb.Scope, error) {

	if resourceID == nil {
		return nil, errors.New("resourceID must not be nil")
	}

	user := &userv1beta1.UserId{
		Idp:      "http://idp.ocis.test",
		OpaqueId: "opaque-user-id-123",
		Type:     userv1beta1.UserType_USER_TYPE_PRIMARY,
	}

	publicShare := &link.PublicShare{
		DisplayName: "some public share name",

		Id: &link.PublicShareId{
			OpaqueId: "opaque-public-link-share-id-123",
		},

		ResourceId: resourceID,

		Permissions: &link.PublicSharePermissions{
			Permissions: &providerv1beta1.ResourcePermissions{
				Stat: true,
			},
		},

		Owner:   user,
		Creator: user,

		Ctime: &typesv1beta1.Timestamp{},
		Mtime: &typesv1beta1.Timestamp{},

		PasswordProtected: false,
		Token:             "token-123",

		Signature: &link.ShareSignature{
			Signature:           "sig-123",
			SignatureExpiration: &typesv1beta1.Timestamp{},
		},

		Expiration: &typesv1beta1.Timestamp{},
	}

	opaqueResource, err := utils.MarshalProtoV1ToJSON(publicShare)
	if err != nil {
		return nil, err
	}

	publicLinkScope := &authpb.Scope{
		Resource: &typesv1beta1.OpaqueEntry{
			Decoder: "json",
			Value:   opaqueResource,
		},
		Role: publicLinkRole,
	}

	return publicLinkScope, nil

}

func Test_StatPublicShareRootByID(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			ResourceId: publicLinkResource,
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_StatPublicShareRootByPath(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-123",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_StatPublicShareFileByRelativeReference(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			ResourceId: &providerv1beta1.ResourceId{
				StorageId: "storage-id-123",
				OpaqueId:  "token-123/foo.txt",
			},
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_StatPublicShareFileByPath(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-123/foo.txt",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_StatFileOutsidePublicShareByPath(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-123/../foo.txt",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_StatFileOnDifferentPublicShareByPath(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	requestedResource := &provider.StatRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-567/../foo.txt",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, requestedResource, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, false)
}

func Test_DeleteFileAsViewer(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_VIEWER,
	)
	assert.NilError(t, err)

	deleteRequest := &provider.DeleteRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-123/foo.txt",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, deleteRequest, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, false)
}

func Test_DeleteFileAsEditor(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_EDITOR,
	)
	assert.NilError(t, err)

	deleteRequest := &provider.DeleteRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &providerv1beta1.Reference{
			Path: "/public/token-123/foo.txt",
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, deleteRequest, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_GetPublicShare(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_EDITOR,
	)
	assert.NilError(t, err)

	publicShareRequest := &link.GetPublicShareRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Token{
				Token: "token-123",
			},
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, publicShareRequest, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, true)
}

func Test_GetDifferentPublicShare(t *testing.T) {
	ctx := context.TODO()

	publicLinkResource := &providerv1beta1.ResourceId{
		StorageId: "storage-id-123",
		OpaqueId:  "file-id-123",
	}

	publicLinkScope, err := getPublicShareScope(
		publicLinkResource,
		authpb.Role_ROLE_EDITOR,
	)
	assert.NilError(t, err)

	publicShareRequest := &link.GetPublicShareRequest{
		Opaque: &typesv1beta1.Opaque{},
		Ref: &link.PublicShareReference{
			Spec: &link.PublicShareReference_Token{
				Token: "token-456",
			},
		},
	}

	allowed, err := publicshareScope(ctx, publicLinkScope, publicShareRequest, &log.Logger)
	assert.NilError(t, err)
	assert.Equal(t, allowed, false)
}
