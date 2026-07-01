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

package scope

import (
	"context"
	"strings"
	"fmt"

	appregistry "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	prefpb "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	stregistry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	sp "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/rs/zerolog"
)

func lightweightAccountScope(_ context.Context, scope *authpb.Scope, resource any, log *zerolog.Logger) (bool, error) {
	// Lightweight accounts have access to resources shared with them.
	// These cannot be resolved from here, but need to be added to the scope from
	// where the call to mint tokens is made.
	switch v := resource.(type) {
	case *collaboration.ListSharesRequest:
		return true, nil
	case *collaboration.ListReceivedSharesRequest:
		return true, nil
	case *ocm.ListOCMSharesRequest:
		return true, nil
	case *ocm.ListReceivedOCMSharesRequest:
		return true, nil
	case *ocm.GetReceivedOCMShareRequest:
		return true, nil
	case *sp.ListStorageSpacesRequest:
		return true, nil
	case *grouppb.GetGroupRequest:
		return true, nil
	case *invitepb.AcceptInviteRequest:
		return true, nil
	case *invitepb.DeleteAcceptedUserRequest:
		return true, nil
	case *invitepb.FindAcceptedUsersRequest:
		return true, nil
	case *invitepb.ForwardInviteRequest:
		return true, nil
	case *invitepb.GenerateInviteTokenRequest:
		return true, nil
	case *invitepb.GetAcceptedUserRequest:
		return true, nil
	case *invitepb.InviteToken:
		return true, nil
	case *invitepb.ListInviteTokensRequest:
		return true, nil
	case *prefpb.GetKeyRequest:
		return true, nil
	case *link.ListPublicSharesRequest:
		return true, nil
	case *stregistry.GetStorageProvidersRequest:
		return true, nil
	case *appregistry.GetAppProvidersRequest:
		return true, nil
	case string:
		return checkLightweightPath(v), nil
	default:
		log.Debug().Str("request_type", fmt.Sprintf("%T", v)).Msg("lightweightAccountScope: request type not supported")
	}
	return false, nil
}

func checkLightweightPath(path string) bool {
	// TODO(lopresti) we need a proper registration mechanism for this
	paths := []string{
		"/app/new",
		"/app/open",
		"/archiver",
		"/dataprovider",
		"/data",
		"/projects",
		"/graph",
		"/ocs/v2.php/apps/files_sharing/api/v1/shares",
		"/ocs/v1.php/apps/files_sharing/api/v1/shares",
		"/ocs/v2.php/apps/files_sharing//api/v1/shares",
		"/ocs/v1.php/apps/files_sharing//api/v1/shares",
		"/ocs/v2.php/cloud/capabilities",
		"/ocs/v1.php/cloud/capabilities",
		"/ocs/v2.php/cloud/user",
		"/ocs/v1.php/cloud/user",
		"/ocm-share",
		"/sciencemesh/generate-invite",
		"/sciencemesh/list-invite",
		"/sciencemesh/accept-invite",
		"/sciencemesh/find-accepted-users",
		"/sciencemesh/delete-accepted-user",
		"/sciencemesh/list-providers",
		"/sciencemesh/open-in-app",
		"/sciencemesh/federations",
		"/sciencemesh/discover",
		"/sciencemesh/embedded-shares",
		"/sciencemesh/process-embedded-share",
		"/remote.php/webdav",
		"/remote.php/dav/files",
		"/remote.php/dav/spaces",
		"/thumbnails",
	}
	for _, p := range paths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// AddLightweightAccountScope adds the scope to allow access to lightweight user.
func AddLightweightAccountScope(role authpb.Role, scopes map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	ref := &sp.Reference{Path: "/"}
	val, err := utils.MarshalProtoV1ToJSON(ref)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		scopes = make(map[string]*authpb.Scope)
	}
	scopes["lightweight"] = &authpb.Scope{
		Resource: &types.OpaqueEntry{
			Decoder: "json",
			Value:   val,
		},
		Role: role,
	}
	return scopes, nil
}
