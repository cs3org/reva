// Copyright 2018-2020 CERN
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

package publicshare

import (
	"context"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Manager manipulates public shares.
type Manager interface {
	CreatePublicShare(ctx context.Context, u *user.User, md *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error)
	UpdatePublicShare(ctx context.Context, u *user.User, req *link.UpdatePublicShareRequest, g *link.Grant) (*link.PublicShare, error)
	GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) (*link.PublicShare, error)
	ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo) ([]*link.PublicShare, error)
	RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error
	GetPublicShareByToken(ctx context.Context, token, password string) (*link.PublicShare, error)
}
