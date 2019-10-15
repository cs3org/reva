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

package publicshare

import (
	"context"

	authproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/authprovider/v0alpha"
	publicshareproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v0alpha"
	storageproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v0alpha"
)

// Manager manipulates public shares.
type Manager interface {
	CreatePublicShare(ctx context.Context, u *authproviderv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error)
	UpdatePublicShare(ctx context.Context, u *authproviderv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference, g *publicshareproviderv0alphapb.Grant) (*publicshareproviderv0alphapb.PublicShare, error)
	GetPublicShare(ctx context.Context, u *authproviderv0alphapb.User, ref *publicshareproviderv0alphapb.PublicShareReference) (*publicshareproviderv0alphapb.PublicShare, error)
	ListPublicShares(ctx context.Context, u *authproviderv0alphapb.User, md *storageproviderv0alphapb.ResourceInfo) ([]*publicshareproviderv0alphapb.PublicShare, error)
	RevokePublicShare(ctx context.Context, u *authproviderv0alphapb.User, id string) error
	GetPublicShareByToken(ctx context.Context, token string) (*publicshareproviderv0alphapb.PublicShare, error)
}
