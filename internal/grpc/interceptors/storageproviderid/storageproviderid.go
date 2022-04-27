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

const (
	defaultPriority = 200
)

// NewUnary returns a new unary interceptor that trims storageprovider ids from incoming requests and prefixes it in responses
//nolint:gocritic
func NewUnary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

		var providerID string
		switch v := req.(type) {
		case *provider.GetPathRequest:
			v.ResourceId.StorageId, providerID = resourceid.StorageIDUnwrap(v.ResourceId.StorageId)
		}

		res, err := handler(ctx, req)
		if err != nil {
			return res, err
		}
		switch v := res.(type) {
		case *provider.GetPathRequest:
			// nothing to change
			_, _ = v, providerID
		}

		return res, nil
	}
}

// NewStream returns a new server stream interceptor
// that creates the application context.
func NewStream() grpc.StreamServerInterceptor {
	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// TODO: Use ss.RecvMsg() and ss.SendMsg() to send events from a stream
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
