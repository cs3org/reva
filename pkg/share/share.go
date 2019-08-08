// Copyright 2018-2019 CERN
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

	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
)

// Manager is the interface that manipulates shares.
type Manager interface {
	// Create a new share in fn with the given acl.
	Share(ctx context.Context, md *storageproviderv0alphapb.ResourceInfo, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.Share, error)

	// GetShare gets the information for a share by the given ref.
	GetShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) (*usershareproviderv0alphapb.Share, error)

	// Unshare deletes the share pointed by ref.
	Unshare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference) error

	// UpdateShare updates the mode of the given share.
	UpdateShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference, p *usershareproviderv0alphapb.SharePermissions) (*usershareproviderv0alphapb.Share, error)

	// ListShares returns the shares created by the user. If md is provided is not nil,
	// it returns only shares attached to the given resource.
	ListShares(ctx context.Context, md *storageproviderv0alphapb.ResourceInfo) ([]*usershareproviderv0alphapb.Share, error)

	// ListReceivedShares returns the list of shares the user has access.
	ListReceivedShares(ctx context.Context) ([]*usershareproviderv0alphapb.ReceivedShare, error)

	// UpdateReceivedShare updates the received share with share state.
	UpdateReceivedShare(ctx context.Context, ref *usershareproviderv0alphapb.ShareReference, f *usershareproviderv0alphapb.UpdateReceivedShareRequest_UpdateField) (*usershareproviderv0alphapb.ReceivedShare, error)
}
