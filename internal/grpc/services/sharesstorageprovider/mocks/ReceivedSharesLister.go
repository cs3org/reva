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

// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	context "context"

	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"

	grpc "google.golang.org/grpc"

	mock "github.com/stretchr/testify/mock"
)

// ReceivedSharesLister is an autogenerated mock type for the ReceivedSharesLister type
type ReceivedSharesLister struct {
	mock.Mock
}

// ListReceivedShares provides a mock function with given fields: ctx, req, opts
func (_m *ReceivedSharesLister) ListReceivedShares(ctx context.Context, req *collaborationv1beta1.ListReceivedSharesRequest, opts ...grpc.CallOption) (*collaborationv1beta1.ListReceivedSharesResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, req)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *collaborationv1beta1.ListReceivedSharesResponse
	if rf, ok := ret.Get(0).(func(context.Context, *collaborationv1beta1.ListReceivedSharesRequest, ...grpc.CallOption) *collaborationv1beta1.ListReceivedSharesResponse); ok {
		r0 = rf(ctx, req, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*collaborationv1beta1.ListReceivedSharesResponse)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *collaborationv1beta1.ListReceivedSharesRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, req, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
