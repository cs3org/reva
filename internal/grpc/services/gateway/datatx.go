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

package gateway

import (
	"context"
	"encoding/json"
	"path"
	"strconv"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/pkg/errors"
)

func (s *svc) CreateTransfer(ctx context.Context, req *datatx.CreateTransferRequest) (*datatx.CreateTransferResponse, error) {
	granteeOpaqueID := req.GetGrantee().GetUserId().GetOpaqueId()
	granteeIdp := req.GetGrantee().GetUserId().GetIdp()

	// check if invitation has been accepted
	imc, err := pool.GetOCMInviteManagerClient(s.c.OCMInviteManagerEndpoint)
	if err != nil {
		err = errors.Wrap(err, "error getting OCM invite manager client")
		return nil, err
	}
	acceptedUserRes, err := imc.GetAcceptedUser(ctx, &invitepb.GetAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: granteeOpaqueID, Idp: granteeIdp},
	})
	if err != nil {
		err = errors.Wrap(err, "error sending a grpc GetAcceptedUser request")
		return nil, err
	}
	if acceptedUserRes.Status.Code != rpc.Code_CODE_OK {
		return &datatx.CreateTransferResponse{
			Status: acceptedUserRes.Status,
		}, nil
	}

	// verify resource status
	gatewayClient, err := pool.GetGatewayServiceClient(s.c.DataGatewayEndpoint)
	if err != nil {
		err = errors.Wrap(err, "error getting gateway client")
		return nil, err
	}
	hRes, err := gatewayClient.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		err = errors.Wrap(err, "error sending a grpc GetHome request")
		return nil, err
	}
	prefix := hRes.GetPath()
	path := path.Join(prefix, req.GetRef().GetPath())
	statReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: path,
			},
		},
	}
	statRes, err := gatewayClient.Stat(ctx, statReq)
	if err != nil {
		err = errors.Wrap(err, "error sending a grpc Stat request")
		return nil, err
	}
	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			err = errors.Wrap(err, "not found")
			return nil, err
		}
		err = errors.Wrap(err, "grpc Stat request failed")
		return nil, err
	}

	providerInfoResp, err := gatewayClient.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: granteeIdp,
	})
	if err != nil {
		err = errors.Wrap(err, "error sending a grpc Get Info By Domain request")
		return nil, err
	}

	permissions := conversions.PermissionWrite
	resourcePermissions := &provider.ResourcePermissions{
		InitiateFileDownload: true,
	}

	datatxProtocol, err := json.Marshal(
		map[string]interface{}{
			"name": "datatx",
			"options": map[string]string{
				"desired-protocol": "webdav",
			},
		},
	)
	if err != nil {
		err = errors.Wrap(err, "error marshalling protocol data")
		return nil, err
	}

	createShareReq := &ocm.CreateOCMShareRequest{
		Opaque: &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"permissions": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(strconv.Itoa(int(permissions))),
				},
				"name": &types.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(path),
				},
				"protocol": &types.OpaqueEntry{
					Decoder: "json",
					Value:   datatxProtocol,
				},
			},
		},
		ResourceId: statRes.Info.Id,
		Grant: &ocm.ShareGrant{
			Grantee: &provider.Grantee{
				Type: req.GetGrantee().GetType(),
				Id:   &provider.Grantee_UserId{UserId: req.GetGrantee().GetUserId()},
			},
			Permissions: &ocm.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
		RecipientMeshProvider: providerInfoResp.ProviderInfo,
	}

	createShareResponse, err := gatewayClient.CreateOCMShare(ctx, createShareReq)
	if err != nil {
		err = errors.Wrap(err, "error sending a grpc Create OCM Share request")
		return nil, err
	}
	if createShareResponse.Status.Code != rpc.Code_CODE_OK {
		if createShareResponse.Status.Code == rpc.Code_CODE_NOT_FOUND {
			err = errors.Wrap(err, "not found")
			return nil, err
		}
		err = errors.Wrap(err, "grpc Create OCM Share request failed")
		return nil, err
	}

	// We do not return a transfer ID in the datatx pull model when creating a transfer type share
	// TODO fix CS3 CreateTransferResponse message to fit in datatx pull model
	txID := int64(0)
	txStatus := datatx.TxInfo_STATUS_TRANSFER_AWAITING_ACCEPTANCE

	res := &datatx.CreateTransferResponse{
		Status: status.NewOK(ctx),
		TxInfo: &datatx.TxInfo{
			Id: &datatx.TxId{
				OpaqueId: strconv.FormatInt(txID, 10),
			},
			Ref: &storageprovider.Reference{
				Spec: &storageprovider.Reference_Path{
					Path: path,
				},
			},
			Status: txStatus,
		},
	}

	return res, nil
}

func (s *svc) GetTransferStatus(ctx context.Context, req *datatx.GetTransferStatusRequest) (*datatx.GetTransferStatusResponse, error) {
	c, err := pool.GetDataTxClient(s.c.DataTxEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &datatx.GetTransferStatusResponse{
			Status: status.NewInternal(ctx, err, "error getting data transfer client"),
		}, nil
	}

	res, err := c.GetTransferStatus(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetTransferStatus")
	}

	return res, nil
}

func (s *svc) CancelTransfer(ctx context.Context, req *datatx.CancelTransferRequest) (*datatx.CancelTransferResponse, error) {
	c, err := pool.GetDataTxClient(s.c.DataTxEndpoint)
	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetOCMShareProviderClient")
		return &datatx.CancelTransferResponse{
			Status: status.NewInternal(ctx, err, "error getting data transfer client"),
		}, nil
	}

	res, err := c.CancelTransfer(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CancelTransfer")
	}

	return res, nil
}
