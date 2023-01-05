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

// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

// UserConverter is an autogenerated mock type for the UserConverter type
type UserConverter struct {
	mock.Mock
}

// UserIDToUserName provides a mock function with given fields: ctx, userid
func (_m *UserConverter) UserIDToUserName(ctx context.Context, userid *userv1beta1.UserId) (string, error) {
	ret := _m.Called(ctx, userid)

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context, *userv1beta1.UserId) string); ok {
		r0 = rf(ctx, userid)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *userv1beta1.UserId) error); ok {
		r1 = rf(ctx, userid)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UserNameToUserID provides a mock function with given fields: ctx, username
func (_m *UserConverter) UserNameToUserID(ctx context.Context, username string) (*userv1beta1.UserId, error) {
	ret := _m.Called(ctx, username)

	var r0 *userv1beta1.UserId
	if rf, ok := ret.Get(0).(func(context.Context, string) *userv1beta1.UserId); ok {
		r0 = rf(ctx, username)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*userv1beta1.UserId)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, username)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
