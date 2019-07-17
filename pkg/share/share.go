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

	authv0alphapb "github.com/cs3org/go-cs3apis/cs3/auth/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
	usershareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v0alpha"
)

// Manager is the interface that manipulates shares.
type Manager interface {
	// Create a new share in fn with the given acl.
	Share(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.Share, error)

	// GetShare gets the information for a share by the given id.
	GetShare(ctx context.Context, u *authv0alphapb.User, id string) (*usershareproviderv0alphapb.Share, error)

	// Unshare deletes the share pointed by id.
	Unshare(ctx context.Context, u *authv0alphapb.User, id string) error

	// UpdateShare updates the mode of the given share.
	UpdateShare(ctx context.Context, u *authv0alphapb.User, id string, g *usershareproviderv0alphapb.ShareGrant) (*usershareproviderv0alphapb.ShareGrant, error)

	// ListShares returns the shares created by the user. If forPath is not empty,
	// it returns only shares attached to the given path.
	ListShares(ctx context.Context, u *authv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*usershareproviderv0alphapb.ShareGrant, error)

	// ListReceivedShares returns the list of shares the user has access.
	ListReceivedShares(ctx context.Context, u *authv0alphapb.User) ([]*usershareproviderv0alphapb.ShareGrant, error)

	// GetReceivedShare returns the information for the share received with
	// the given id.
	GetReceivedShare(ctx context.Context, u *authv0alphapb.User, id string) (*usershareproviderv0alphapb.ShareGrant, error)

	// RejectReceivedShare rejects the share by the given id.
	RejectReceivedShare(ctx context.Context, u *authv0alphapb.User, id string) error
}
