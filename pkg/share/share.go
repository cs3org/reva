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

package share

import (
	"context"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/utils"
	"google.golang.org/genproto/protobuf/field_mask"
)

//go:generate mockery -name Manager

// Manager is the interface that manipulates shares.
type Manager interface {
	// Create a new share in fn with the given acl.
	Share(ctx context.Context, md *provider.ResourceInfo, g *collaboration.ShareGrant) (*collaboration.Share, error)

	// GetShare gets the information for a share by the given ref.
	GetShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.Share, error)

	// Unshare deletes the share pointed by ref.
	Unshare(ctx context.Context, ref *collaboration.ShareReference) error

	// UpdateShare updates the mode of the given share.
	UpdateShare(ctx context.Context, ref *collaboration.ShareReference, req *collaboration.UpdateShareRequest) (*collaboration.Share, error)

	// ListShares returns the shares created by the user. If md is provided is not nil,
	// it returns only shares attached to the given resource.
	ListShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.Share, error)

	// ListReceivedShares returns the list of shares the user has access to.
	ListReceivedShares(ctx context.Context, filters []*collaboration.Filter) ([]*collaboration.ReceivedShare, error)

	// GetReceivedShare returns the information for a received share.
	GetReceivedShare(ctx context.Context, ref *collaboration.ShareReference) (*collaboration.ReceivedShare, error)

	// UpdateReceivedShare updates the received share with share state.
	UpdateReceivedShare(ctx context.Context, share *collaboration.ReceivedShare, fieldMask *field_mask.FieldMask) (*collaboration.ReceivedShare, error)
}

// GroupGranteeFilter is an abstraction for creating filter by grantee type group.
func GroupGranteeFilter() *collaboration.Filter {
	return &collaboration.Filter{
		Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
		Term: &collaboration.Filter_GranteeType{
			GranteeType: provider.GranteeType_GRANTEE_TYPE_GROUP,
		},
	}
}

// UserGranteeFilter is an abstraction for creating filter by grantee type user.
func UserGranteeFilter() *collaboration.Filter {
	return &collaboration.Filter{
		Type: collaboration.Filter_TYPE_GRANTEE_TYPE,
		Term: &collaboration.Filter_GranteeType{
			GranteeType: provider.GranteeType_GRANTEE_TYPE_USER,
		},
	}
}

// ResourceIDFilter is an abstraction for creating filter by resource id.
func ResourceIDFilter(id *provider.ResourceId) *collaboration.Filter {
	return &collaboration.Filter{
		Type: collaboration.Filter_TYPE_RESOURCE_ID,
		Term: &collaboration.Filter_ResourceId{
			ResourceId: id,
		},
	}
}

// IsCreatedByUser checks if the user is the owner or creator of the share.
func IsCreatedByUser(share *collaboration.Share, user *userv1beta1.User) bool {
	return utils.UserEqual(user.Id, share.Owner) || utils.UserEqual(user.Id, share.Creator)
}

// IsGrantedToUser checks if the user is a grantee of the share. Either by a user grant or by a group grant.
func IsGrantedToUser(share *collaboration.Share, user *userv1beta1.User) bool {
	if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, share.Grantee.GetUserId()) {
		return true
	}
	if share.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
		// check if any of the user's group is the grantee of the share
		for _, g := range user.Groups {
			if g == share.Grantee.GetGroupId().OpaqueId {
				return true
			}
		}
	}
	return false
}

// MatchesFilter tests if the share passes the filter.
func MatchesFilter(share *collaboration.Share, filter *collaboration.Filter) bool {
	switch filter.Type {
	case collaboration.Filter_TYPE_RESOURCE_ID:
		return utils.ResourceIDEqual(share.ResourceId, filter.GetResourceId())
	case collaboration.Filter_TYPE_GRANTEE_TYPE:
		return share.Grantee.Type == filter.GetGranteeType()
	case collaboration.Filter_TYPE_EXCLUDE_DENIALS:
		return share.Permissions.Permissions.DenyGrant
	default:
		return false
	}
}

// MatchesAnyFilter checks if the share passes at least one of the given filters.
func MatchesAnyFilter(share *collaboration.Share, filters []*collaboration.Filter) bool {
	for _, f := range filters {
		if MatchesFilter(share, f) {
			return true
		}
	}
	return false
}

// MatchesFilters checks if the share passes the given filters.
// Filters of the same type form a disjuntion, a logical OR. Filters of separate type form a conjunction, a logical AND.
// Here is an example:
// (resource_id=1 OR resource_id=2) AND (grantee_type=USER OR grantee_type=GROUP).
func MatchesFilters(share *collaboration.Share, filters []*collaboration.Filter) bool {
	grouped := GroupFiltersByType(filters)
	for _, f := range grouped {
		if !MatchesAnyFilter(share, f) {
			return false
		}
	}
	return true
}

// GroupFiltersByType groups the given filters and returns a map using the filter type as the key.
func GroupFiltersByType(filters []*collaboration.Filter) map[collaboration.Filter_Type][]*collaboration.Filter {
	grouped := make(map[collaboration.Filter_Type][]*collaboration.Filter)
	for _, f := range filters {
		grouped[f.Type] = append(grouped[f.Type], f)
	}
	return grouped
}
