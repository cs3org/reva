// Copyright 2018-2026 CERN
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

package grpc

import (
	"context"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	"github.com/cs3org/reva/v3/pkg/share"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type shares struct{ b *Backend }

func shareRef(id string) *collaboration.ShareReference {
	return &collaboration.ShareReference{Spec: &collaboration.ShareReference_Id{Id: &collaboration.ShareId{OpaqueId: id}}}
}

func (s shares) List(ctx context.Context, resource *provider.ResourceId) ([]*collaboration.Share, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	req := &collaboration.ListSharesRequest{}
	if resource != nil {
		req.Filters = []*collaboration.Filter{share.ResourceIDFilter(resource)}
	}
	res, err := check(c.ListShares(ctx, req))
	if err != nil {
		return nil, err
	}
	return res.Shares, nil
}

func (s shares) Get(ctx context.Context, id string) (*collaboration.Share, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.GetShare(ctx, &collaboration.GetShareRequest{Ref: shareRef(id)}))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (s shares) Create(ctx context.Context, req *collaboration.CreateShareRequest) (*collaboration.Share, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.CreateShare(ctx, req))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (s shares) Update(ctx context.Context, req *collaboration.UpdateShareRequest) (*collaboration.Share, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.UpdateShare(ctx, req))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (s shares) Remove(ctx context.Context, id string) error {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return err
	}
	_, err = check(c.RemoveShare(ctx, &collaboration.RemoveShareRequest{Ref: shareRef(id)}))
	return err
}

func (s shares) Received(ctx context.Context) ([]*collaboration.ReceivedShare, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Shares, nil
}

func (s shares) UpdateReceived(ctx context.Context, id string, state collaboration.ShareState) (*collaboration.ReceivedShare, error) {
	return s.updateReceived(ctx, id, "state", func(rs *collaboration.ReceivedShare) { rs.State = state })
}

func (s shares) HideReceived(ctx context.Context, id string) error {
	_, err := s.updateReceived(ctx, id, "hidden", func(rs *collaboration.ReceivedShare) { rs.Hidden = true })
	return err
}

// updateReceived changes one field of a received share. The update takes
// the whole share, so it reads it first.
func (s shares) updateReceived(ctx context.Context, id, field string, set func(*collaboration.ReceivedShare)) (*collaboration.ReceivedShare, error) {
	c, ctx, err := s.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	got, err := check(c.GetReceivedShare(ctx, &collaboration.GetReceivedShareRequest{Ref: shareRef(id)}))
	if err != nil {
		return nil, err
	}
	set(got.Share)
	res, err := check(c.UpdateReceivedShare(ctx, &collaboration.UpdateReceivedShareRequest{
		Share:      got.Share,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{field}},
	}))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

type links struct{ b *Backend }

// List filters on the creator, which needs the user id of the token.
func (l links) List(ctx context.Context, resource *provider.ResourceId) ([]*link.PublicShare, error) {
	me, err := l.b.Users().Me(ctx)
	if err != nil {
		return nil, err
	}
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	filters := []*link.ListPublicSharesRequest_Filter{{
		Type: link.ListPublicSharesRequest_Filter_TYPE_CREATOR,
		Term: &link.ListPublicSharesRequest_Filter_Creator{Creator: me.Id},
	}}
	if resource != nil {
		filters = append(filters, publicshare.ResourceIDFilter(resource))
	}
	res, err := check(c.ListPublicShares(ctx, &link.ListPublicSharesRequest{Filters: filters}))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (l links) Get(ctx context.Context, ref *link.PublicShareReference) (*link.PublicShare, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: ref}))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (l links) Create(ctx context.Context, req *link.CreatePublicShareRequest) (*link.PublicShare, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.CreatePublicShare(ctx, req))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (l links) Update(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.PublicShare, error) {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	res, err := check(c.UpdatePublicShare(ctx, req))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (l links) Remove(ctx context.Context, ref *link.PublicShareReference) error {
	c, ctx, err := l.b.gw(ctx)
	if err != nil {
		return err
	}
	_, err = check(c.RemovePublicShare(ctx, &link.RemovePublicShareRequest{Ref: ref}))
	return err
}
