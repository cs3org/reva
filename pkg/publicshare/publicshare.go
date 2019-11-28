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

	publicshareproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/publicshareprovider/v1beta1"
	storageproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/storageprovider/v1beta1"
	userproviderv1beta1pb "github.com/cs3org/go-cs3apis/cs3/userprovider/v1beta1"
)

// Manager manipulates public shares.
type Manager interface {
	CreatePublicShare(ctx context.Context, u *userproviderv1beta1pb.User, md *storageproviderv1beta1pb.ResourceInfo, g *publicshareproviderv1beta1pb.Grant) (*publicshareproviderv1beta1pb.PublicShare, error)
	UpdatePublicShare(ctx context.Context, u *userproviderv1beta1pb.User, ref *publicshareproviderv1beta1pb.PublicShareReference, g *publicshareproviderv1beta1pb.Grant) (*publicshareproviderv1beta1pb.PublicShare, error)
	GetPublicShare(ctx context.Context, u *userproviderv1beta1pb.User, ref *publicshareproviderv1beta1pb.PublicShareReference) (*publicshareproviderv1beta1pb.PublicShare, error)
	ListPublicShares(ctx context.Context, u *userproviderv1beta1pb.User, md *storageproviderv1beta1pb.ResourceInfo) ([]*publicshareproviderv1beta1pb.PublicShare, error)
	RevokePublicShare(ctx context.Context, u *userproviderv1beta1pb.User, id string) error
	GetPublicShareByToken(ctx context.Context, token string) (*publicshareproviderv1beta1pb.PublicShare, error)
}
