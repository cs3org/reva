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

package storageproviderid

import (
	"context"

	"google.golang.org/grpc"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	v1beta12 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/pkg/utils/resourceid"
)

// NewUnary returns a new unary interceptor that trims storageprovider ids from incoming requests and prefixes it in responses
//nolint:gocritic
func NewUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		req, providerID := adjustRequestStorageID(req)

		res, err := handler(ctx, req)
		if err != nil {
			return res, err
		}

		// we can stop if we weren't successful
		if s, ok := res.(su); ok && !isSuccess(s) {
			return res, nil
		}

		return adjustResponseStorageID(res, providerID), nil
	}
}

// NewStream returns a new server stream interceptor
// that creates the application context.
func NewStream() grpc.StreamServerInterceptor {
	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// TODO: Use ss.RecvMsg() and ss.SendMsg() to send events from a stream
		// Handle:
		//	*provider.ListContainerStreamRequest
		//	*provider.ListRecycleStreamRequest
		return handler(srv, ss)
	}
	return interceptor
}

// common interface to all responses
type su interface {
	GetStatus() *v1beta12.Status
}

func isSuccess(res su) bool {
	return res.GetStatus().Code == rpc.Code_CODE_OK
}

func adjustResponseStorageID(res interface{}, providerID string) interface{} {
	switch v := res.(type) {
	case *provider.ListStorageSpacesResponse:
		for _, s := range v.StorageSpaces {
			s.Id.OpaqueId = resourceid.StorageIDWrap(s.Id.GetOpaqueId(), providerID)
		}
	case *provider.UpdateStorageSpaceResponse:
		v.StorageSpace.Id.OpaqueId = resourceid.StorageIDWrap(v.StorageSpace.Id.GetOpaqueId(), providerID)
	case *provider.StatResponse:
		v.Info.Id.StorageId = resourceid.StorageIDWrap(v.Info.Id.GetStorageId(), providerID)
	case *provider.ListContainerResponse:
		for _, i := range v.Infos {
			i.Id.StorageId = resourceid.StorageIDWrap(i.Id.GetStorageId(), providerID)
		}
	case *provider.ListRecycleResponse:
		for _, i := range v.RecycleItems {
			i.Ref.ResourceId.StorageId = resourceid.StorageIDWrap(i.Ref.GetResourceId().GetStorageId(), providerID)
		}
	}

	return res
}

func adjustRequestStorageID(req interface{}) (interface{}, string) {
	var providerID string
	switch v := req.(type) {
	case *provider.GetPathRequest:
		if v.GetResourceId() != nil {
			v.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.ResourceId.StorageId)
		}
	case *provider.SetArbitraryMetadataRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.UnsetArbitraryMetadataRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.SetLockRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.GetLockRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.RefreshLockRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.UnlockRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.InitiateFileDownloadRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.InitiateFileUploadRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.ListStorageSpacesRequest:
		for i, f := range v.Filters {
			if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
				id, pid := resourceid.StorageIDUnwrap(f.GetId().GetOpaqueId())
				v.Filters[i].Term = &provider.ListStorageSpacesRequest_Filter_Id{Id: &provider.StorageSpaceId{OpaqueId: id}}
				providerID = pid
				break
			}
		}
	case *provider.UpdateStorageSpaceRequest:
		if v.GetStorageSpace().GetId() != nil {
			v.StorageSpace.Id.OpaqueId, providerID = resourceid.StorageIDUnwrap(v.StorageSpace.Id.OpaqueId)
		}
	case *provider.DeleteStorageSpaceRequest:
		if v.GetId() != nil {
			v.Id.OpaqueId, providerID = resourceid.StorageIDUnwrap(v.Id.OpaqueId)
		}
	case *provider.CreateContainerRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.TouchFileRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.DeleteRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.MoveRequest:
		if v.Source.GetResourceId() != nil {
			v.Source.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Source.ResourceId.StorageId)
		}
		if v.Destination.GetResourceId() != nil {
			v.Destination.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Destination.ResourceId.StorageId)
		}
	case *provider.StatRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.ListContainerRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.ListFileVersionsRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.RestoreFileVersionRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.ListRecycleRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.RestoreRecycleItemRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.PurgeRecycleRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.ListGrantsRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.DenyGrantRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.AddGrantRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.UpdateGrantRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.RemoveGrantRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.CreateReferenceRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.CreateSymlinkRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}
	case *provider.GetQuotaRequest:
		if v.Ref.GetResourceId() != nil {
			v.Ref.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.Ref.ResourceId.StorageId)
		}

	}

	return req, providerID
}
