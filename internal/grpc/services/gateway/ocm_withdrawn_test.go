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

package gateway

import (
	"context"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
)

type refusingOCMProvider struct {
	ocm.OcmAPIClient
	share *ocm.ReceivedShare
}

func (p *refusingOCMProvider) GetReceivedOCMShare(context.Context, *ocm.GetReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.GetReceivedOCMShareResponse, error) {
	return &ocm.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: p.share}, nil
}

func (p *refusingOCMProvider) UpdateReceivedOCMShare(context.Context, *ocm.UpdateReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.UpdateReceivedOCMShareResponse, error) {
	return &ocm.UpdateReceivedOCMShareResponse{Status: &rpc.Status{
		Code: rpc.Code_CODE_FAILED_PRECONDITION, Message: share.WithdrawnShareMessage,
	}}, nil
}

// A refused accept of a transfer share returns the refusal and never
// reaches the transfer path (which would need a datatx service and a token).
func TestRefusedReceivedOCMShareUpdateHasNoSideEffects(t *testing.T) {
	stampOCMShareProvider(&refusingOCMProvider{share: &ocm.ReceivedShare{
		Id:    &ocm.ShareId{OpaqueId: "7"},
		State: ocm.ShareState_SHARE_STATE_INVALID,
		Protocols: []*ocm.Protocol{share.NewWebDAVProtocol("https://a.example.org/dav", "secret", nil,
			[]ocm.AccessType{ocm.AccessType_ACCESS_TYPE_DATATX}, nil)},
	}})
	res, err := (&svc{}).UpdateReceivedOCMShare(context.Background(), &ocm.UpdateReceivedOCMShareRequest{
		Share:      &ocm.ReceivedShare{Id: &ocm.ShareId{OpaqueId: "7"}, State: ocm.ShareState_SHARE_STATE_ACCEPTED},
		UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_FAILED_PRECONDITION || res.GetStatus().GetMessage() != share.WithdrawnShareMessage {
		t.Fatalf("status %v", res.GetStatus())
	}
}
