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

package eventsmiddleware

import (
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/v2/pkg/events"
)

// ShareCreated converts the response to an event
func ShareCreated(r *collaboration.CreateShareResponse) events.ShareCreated {
	return events.ShareCreated{
		Sharer:         r.Share.Creator,
		GranteeUserID:  r.Share.GetGrantee().GetUserId(),
		GranteeGroupID: r.Share.GetGrantee().GetGroupId(),
		ItemID:         r.Share.ResourceId,
		CTime:          r.Share.Ctime,
	}
}

// ShareRemoved converts the response to an event
func ShareRemoved(r *collaboration.RemoveShareResponse, req *collaboration.RemoveShareRequest) events.ShareRemoved {
	return events.ShareRemoved{
		ShareID:  req.Ref.GetId(),
		ShareKey: req.Ref.GetKey(),
	}
}

// ShareUpdated converts the response to an event
func ShareUpdated(r *collaboration.UpdateShareResponse, req *collaboration.UpdateShareRequest) events.ShareUpdated {
	updated := ""
	if req.Field.GetPermissions() != nil {
		updated = "permissions"
	} else if req.Field.GetDisplayName() != "" {
		updated = "displayname"
	}
	return events.ShareUpdated{
		ShareID:        r.Share.Id,
		ItemID:         r.Share.ResourceId,
		Permissions:    r.Share.Permissions,
		GranteeUserID:  r.Share.GetGrantee().GetUserId(),
		GranteeGroupID: r.Share.GetGrantee().GetGroupId(),
		Sharer:         r.Share.Creator,
		MTime:          r.Share.Mtime,
		Updated:        updated,
	}
}

// LinkCreated converts the response to an event
func LinkCreated(r *link.CreatePublicShareResponse) events.LinkCreated {
	return events.LinkCreated{
		ShareID:           r.Share.Id,
		Sharer:            r.Share.Creator,
		ItemID:            r.Share.ResourceId,
		Permissions:       r.Share.Permissions,
		DisplayName:       r.Share.DisplayName,
		Expiration:        r.Share.Expiration,
		PasswordProtected: r.Share.PasswordProtected,
		CTime:             r.Share.Ctime,
		Token:             r.Share.Token,
	}
}

// LinkUpdated converts the response to an event
func LinkUpdated(r *link.UpdatePublicShareResponse, req *link.UpdatePublicShareRequest) events.LinkUpdated {
	return events.LinkUpdated{
		ShareID:           r.Share.Id,
		Sharer:            r.Share.Creator,
		ItemID:            r.Share.ResourceId,
		Permissions:       r.Share.Permissions,
		DisplayName:       r.Share.DisplayName,
		Expiration:        r.Share.Expiration,
		PasswordProtected: r.Share.PasswordProtected,
		CTime:             r.Share.Ctime,
		Token:             r.Share.Token,
		FieldUpdated:      link.UpdatePublicShareRequest_Update_Type_name[int32(req.Update.GetType())],
	}
}
