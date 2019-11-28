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

	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	usershareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/usershareprovider/v1beta1"
)

// Manager is the interface that manipulates shares.
type Manager interface {
	// Create a new share in fn with the given acl.
	Share(ctx context.Context, md *storageproviderv1beta1pb.ResourceInfo, g *usershareproviderv1beta1pb.ShareGrant) (*usershareproviderv1beta1pb.Share, error)

	// GetShare gets the information for a share by the given ref.
	GetShare(ctx context.Context, ref *usershareproviderv1beta1pb.ShareReference) (*usershareproviderv1beta1pb.Share, error)

	// Unshare deletes the share pointed by ref.
	Unshare(ctx context.Context, ref *usershareproviderv1beta1pb.ShareReference) error

	// UpdateShare updates the mode of the given share.
	UpdateShare(ctx context.Context, ref *usershareproviderv1beta1pb.ShareReference, p *usershareproviderv1beta1pb.SharePermissions) (*usershareproviderv1beta1pb.Share, error)

	// ListShares returns the shares created by the user. If md is provided is not nil,
	// it returns only shares attached to the given resource.
	ListShares(ctx context.Context, filters []*usershareproviderv1beta1pb.ListSharesRequest_Filter) ([]*usershareproviderv1beta1pb.Share, error)

	// ListReceivedShares returns the list of shares the user has access.
	ListReceivedShares(ctx context.Context) ([]*usershareproviderv1beta1pb.ReceivedShare, error)

	// GetReceivedShare returns the information for a received share the user has access.
	GetReceivedShare(ctx context.Context, ref *usershareproviderv1beta1pb.ShareReference) (*usershareproviderv1beta1pb.ReceivedShare, error)

	// UpdateReceivedShare updates the received share with share state.
	UpdateReceivedShare(ctx context.Context, ref *usershareproviderv1beta1pb.ShareReference, f *usershareproviderv1beta1pb.UpdateReceivedShareRequest_UpdateField) (*usershareproviderv1beta1pb.ReceivedShare, error)
}
