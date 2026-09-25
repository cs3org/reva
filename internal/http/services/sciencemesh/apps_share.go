// Copyright 2018-2024 CERN
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

package sciencemesh

import (
	"context"

	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/service"
)

func (h *appsHandler) receivedWebapp(
	ctx context.Context,
	id *ocmpb.ShareId,
) (*ocmpb.ReceivedShare, *ocmpb.WebappProtocol, error) {
	gatewayClient, err := service.Gateway(ctx)
	if err != nil {
		return nil, nil, err
	}
	if gatewayClient == nil {
		return nil, nil, errtypes.InternalError("gateway client is not available")
	}

	res, err := gatewayClient.GetReceivedOCMShare(ctx, &ocmpb.GetReceivedOCMShareRequest{
		Ref: &ocmpb.ShareReference{
			Spec: &ocmpb.ShareReference_Id{
				Id: id,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if res == nil || res.Status == nil {
		return nil, nil, errtypes.InternalError("missing share response")
	}
	switch res.Status.Code {
	case rpcv1beta1.Code_CODE_OK:
	case rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound("received share not found")
	case rpcv1beta1.Code_CODE_PERMISSION_DENIED:
		return nil, nil, errtypes.PermissionDenied("received share access denied")
	case rpcv1beta1.Code_CODE_UNAUTHENTICATED:
		return nil, nil, errtypes.InvalidCredentials("received share unauthenticated")
	default:
		return nil, nil, errtypes.InternalError("received share lookup failed")
	}
	if res.Share == nil {
		return nil, nil, errtypes.NotFound("missing share")
	}

	webapp, err := requireWebappProtocol(res.Share.Protocols)
	if err != nil {
		return nil, nil, err
	}
	return res.Share, webapp, nil
}

// requireWebappProtocol returns the single stored webapp descriptor.
// Missing, malformed, and duplicate webapp entries are rejected.
func requireWebappProtocol(protocols []*ocmpb.Protocol) (*ocmpb.WebappProtocol, error) {
	found := []*ocmpb.WebappProtocol{}
	for _, p := range protocols {
		if p == nil {
			continue
		}
		opts, ok := p.Term.(*ocmpb.Protocol_WebappOptions)
		if !ok {
			continue
		}
		if opts == nil || opts.WebappOptions == nil {
			return nil, errtypes.BadRequest("webapp protocol missing options")
		}
		found = append(found, opts.WebappOptions)
	}
	switch len(found) {
	case 0:
		return nil, errtypes.BadRequest("share does not contain webapp protocol")
	case 1:
		return found[0], nil
	default:
		return nil, errtypes.BadRequest("duplicate webapp protocol")
	}
}
