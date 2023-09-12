// Copyright 2018-2023 CERN
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

package notrashbin

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc"
	rstatus "github.com/cs3org/reva/pkg/rgrpc/status"
	"google.golang.org/grpc"
)

const (
	defaultPriority = 200
)

func init() {
	rgrpc.RegisterUnaryInterceptor("notrashbin", NewUnary)
}

// NewUnary returns a new unary interceptor
// that checks grpc calls and blocks write requests.
func NewUnary(_ map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		switch req.(type) {
		case *provider.ListContainerRequest:
			resp, err := handler(ctx, req)
			if listResp, ok := resp.(*provider.ListContainerResponse); ok && listResp.Infos != nil {
				for _, info := range listResp.Infos {
					if info.PermissionSet != nil {
						info.PermissionSet.ListRecycle = false
						info.PermissionSet.RestoreRecycleItem = false
						info.PermissionSet.PurgeRecycle = false
					}
				}
			}
			return resp, err
		case *provider.StatRequest:
			resp, err := handler(ctx, req)
			if statResp, ok := resp.(*provider.StatResponse); ok && statResp.Info != nil && statResp.Info.PermissionSet != nil {
				statResp.Info.PermissionSet.ListRecycle = false
				statResp.Info.PermissionSet.RestoreRecycleItem = false
				statResp.Info.PermissionSet.PurgeRecycle = false
			}
			return resp, err
		case *provider.ListRecycleRequest:
			return &provider.ListRecycleResponse{
				Status: rstatus.NewPermissionDenied(ctx, nil, "permission denied: tried to list recycle bin on a no trashbin storage"),
			}, nil
		case *provider.RestoreRecycleItemRequest:
			return &provider.RestoreRecycleItemResponse{
				Status: rstatus.NewPermissionDenied(ctx, nil, "permission denied: tried to restore recycle item on a no trashbin storage"),
			}, nil
		case *provider.PurgeRecycleRequest:
			return &provider.PurgeRecycleResponse{
				Status: rstatus.NewPermissionDenied(ctx, nil, "permission denied: tried to purge recycle bin on a no trashbin storage"),
			}, nil
		default:
			return handler(ctx, req)
		}
	}, defaultPriority, nil
}
