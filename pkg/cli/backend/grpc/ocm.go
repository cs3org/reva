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

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/utils"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

type ocmArea struct{ b *Backend }

func ocmShareRef(id string) *ocm.ShareReference {
	return &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: &ocm.ShareId{OpaqueId: id}}}
}

func (o ocmArea) CreateInvite(ctx context.Context, description string) (*invitepb.InviteToken, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.GenerateInviteToken, &invitepb.GenerateInviteTokenRequest{Description: description})
	if err != nil {
		return nil, err
	}
	return res.InviteToken, nil
}

func (o ocmArea) Invites(ctx context.Context) ([]*invitepb.InviteToken, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.ListInviteTokens, &invitepb.ListInviteTokensRequest{})
	if err != nil {
		return nil, err
	}
	return res.InviteTokens, nil
}

func (o ocmArea) AcceptInvite(ctx context.Context, token, domain string) error {
	c, ctx, err := o.b.gw(ctx)
	if err != nil {
		return err
	}
	info, err := check(c.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{Domain: domain}))
	if err != nil {
		return err
	}
	_, err = check(c.ForwardInvite(ctx, &invitepb.ForwardInviteRequest{
		InviteToken:          &invitepb.InviteToken{Token: token},
		OriginSystemProvider: info.ProviderInfo,
	}))
	return err
}

func (o ocmArea) Contacts(ctx context.Context, filter string) ([]*userpb.User, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.FindAcceptedUsers, &invitepb.FindAcceptedUsersRequest{Filter: filter})
	if err != nil {
		return nil, err
	}
	return res.AcceptedUsers, nil
}

func (o ocmArea) RemoveContact(ctx context.Context, id *userpb.UserId) error {
	_, err := call(ctx, o.b, gateway.GatewayAPIClient.DeleteAcceptedUser, &invitepb.DeleteAcceptedUserRequest{RemoteUserId: id})
	return err
}

func (o ocmArea) Providers(ctx context.Context) ([]*ocmprovider.ProviderInfo, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.ListAllProviders, &ocmprovider.ListAllProvidersRequest{})
	if err != nil {
		return nil, err
	}
	return res.Providers, nil
}

func (o ocmArea) CreateShare(ctx context.Context, ref *provider.Reference, grantee *userpb.UserId, methods []*ocm.AccessMethod) (*ocm.Share, error) {
	me, err := o.b.Users().Me(ctx)
	if err != nil {
		return nil, err
	}
	c, ctx, err := o.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	info, err := check(c.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{Domain: grantee.Idp}))
	if err != nil {
		return nil, err
	}
	userFilter, err := utils.MarshalProtoV1ToJSON(me.Id)
	if err != nil {
		return nil, err
	}
	remote, err := check(c.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: grantee.OpaqueId, Idp: grantee.Idp, Type: userpb.UserType_USER_TYPE_FEDERATED},
		Opaque:       &types.Opaque{Map: map[string]*types.OpaqueEntry{"user-filter": {Decoder: "json", Value: userFilter}}},
	}))
	if err != nil {
		return nil, err
	}
	stat, err := check(c.Stat(ctx, &provider.StatRequest{Ref: ref}))
	if err != nil {
		return nil, err
	}
	res, err := check(c.CreateOCMShare(ctx, &ocm.CreateOCMShareRequest{
		ResourceId: stat.Info.Id,
		// Only user grantees are supported.
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: remote.RemoteUser.Id},
		},
		RecipientMeshProvider: info.ProviderInfo,
		AccessMethods:         methods,
	}))
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (o ocmArea) Shares(ctx context.Context, resource *provider.ResourceId) ([]*ocm.Share, error) {
	req := &ocm.ListOCMSharesRequest{}
	if resource != nil {
		req.Filters = []*ocm.ListOCMSharesRequest_Filter{ocmshare.ResourceIDFilter(resource)}
	}
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.ListOCMShares, req)
	if err != nil {
		return nil, err
	}
	return res.Shares, nil
}

func (o ocmArea) GetShare(ctx context.Context, id string) (*ocm.Share, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.GetOCMShare, &ocm.GetOCMShareRequest{Ref: ocmShareRef(id)})
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

func (o ocmArea) UpdateShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) error {
	_, err := call(ctx, o.b, gateway.GatewayAPIClient.UpdateOCMShare, req)
	return err
}

func (o ocmArea) RemoveShare(ctx context.Context, id string) error {
	_, err := call(ctx, o.b, gateway.GatewayAPIClient.RemoveOCMShare, &ocm.RemoveOCMShareRequest{Ref: ocmShareRef(id)})
	return err
}

func (o ocmArea) Received(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.ListReceivedOCMShares, &ocm.ListReceivedOCMSharesRequest{})
	if err != nil {
		return nil, err
	}
	return res.Shares, nil
}

func (o ocmArea) GetReceived(ctx context.Context, id string) (*ocm.ReceivedShare, error) {
	res, err := call(ctx, o.b, gateway.GatewayAPIClient.GetReceivedOCMShare, &ocm.GetReceivedOCMShareRequest{Ref: ocmShareRef(id)})
	if err != nil {
		return nil, err
	}
	return res.Share, nil
}

// UpdateReceived reads the share first, because the update takes all of it.
func (o ocmArea) UpdateReceived(ctx context.Context, id string, state ocm.ShareState) error {
	rs, err := o.GetReceived(ctx, id)
	if err != nil {
		return err
	}
	c, ctx, err := o.b.gw(ctx)
	if err != nil {
		return err
	}
	rs.State = state
	_, err = check(c.UpdateReceivedOCMShare(ctx, &ocm.UpdateReceivedOCMShareRequest{
		Share:      rs,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	}))
	return err
}

func (b *Backend) ListTransfers(ctx context.Context, shareID string) ([]*datatx.TxInfo, error) {
	req := &datatx.ListTransfersRequest{}
	if shareID != "" {
		req.Filters = []*datatx.ListTransfersRequest_Filter{{
			Type: datatx.ListTransfersRequest_Filter_TYPE_SHARE_ID,
			Term: &datatx.ListTransfersRequest_Filter_ShareId{ShareId: &ocm.ShareId{OpaqueId: shareID}},
		}}
	}
	res, err := call(ctx, b, gateway.GatewayAPIClient.ListTransfers, req)
	if err != nil {
		return nil, err
	}
	return res.Transfers, nil
}

func (b *Backend) TransferStatus(ctx context.Context, id string) (*datatx.TxInfo, error) {
	res, err := call(ctx, b, gateway.GatewayAPIClient.GetTransferStatus, &datatx.GetTransferStatusRequest{TxId: &datatx.TxId{OpaqueId: id}})
	if err != nil {
		return nil, err
	}
	return res.TxInfo, nil
}

func (b *Backend) CancelTransfer(ctx context.Context, id string) (*datatx.TxInfo, error) {
	res, err := call(ctx, b, gateway.GatewayAPIClient.CancelTransfer, &datatx.CancelTransferRequest{TxId: &datatx.TxId{OpaqueId: id}})
	if err != nil {
		return nil, err
	}
	return res.TxInfo, nil
}

func (b *Backend) RetryTransfer(ctx context.Context, id string) (*datatx.TxInfo, error) {
	res, err := call(ctx, b, gateway.GatewayAPIClient.RetryTransfer, &datatx.RetryTransferRequest{TxId: &datatx.TxId{OpaqueId: id}})
	if err != nil {
		return nil, err
	}
	return res.TxInfo, nil
}
