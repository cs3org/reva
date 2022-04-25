// Copyright 2018-2022 CERN
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

// Code generated by mockery v2.11.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	testing "testing"
)

// Storage is an autogenerated mock type for the Storage type
type Storage struct {
	mock.Mock
}

// Backend provides a mock function with given fields:
func (_m *Storage) Backend() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// CreateSymlink provides a mock function with given fields: ctx, oldname, newname
func (_m *Storage) CreateSymlink(ctx context.Context, oldname string, newname string) error {
	ret := _m.Called(ctx, oldname, newname)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, oldname, newname)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Delete provides a mock function with given fields: ctx, path
func (_m *Storage) Delete(ctx context.Context, path string) error {
	ret := _m.Called(ctx, path)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, path)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Init provides a mock function with given fields: ctx, name
func (_m *Storage) Init(ctx context.Context, name string) error {
	ret := _m.Called(ctx, name)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MakeDirIfNotExist provides a mock function with given fields: ctx, name
func (_m *Storage) MakeDirIfNotExist(ctx context.Context, name string) error {
	ret := _m.Called(ctx, name)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReadDir provides a mock function with given fields: ctx, path
func (_m *Storage) ReadDir(ctx context.Context, path string) ([]string, error) {
	ret := _m.Called(ctx, path)

	var r0 []string
	if rf, ok := ret.Get(0).(func(context.Context, string) []string); ok {
		r0 = rf(ctx, path)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, path)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ResolveSymlink provides a mock function with given fields: ctx, name
func (_m *Storage) ResolveSymlink(ctx context.Context, name string) (string, error) {
	ret := _m.Called(ctx, name)

	var r0 string
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SimpleDownload provides a mock function with given fields: ctx, path
func (_m *Storage) SimpleDownload(ctx context.Context, path string) ([]byte, error) {
	ret := _m.Called(ctx, path)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(context.Context, string) []byte); ok {
		r0 = rf(ctx, path)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, path)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SimpleUpload provides a mock function with given fields: ctx, uploadpath, content
func (_m *Storage) SimpleUpload(ctx context.Context, uploadpath string, content []byte) error {
	ret := _m.Called(ctx, uploadpath, content)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []byte) error); ok {
		r0 = rf(ctx, uploadpath, content)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewStorage creates a new instance of Storage. It also registers a cleanup function to assert the mocks expectations.
func NewStorage(t testing.TB) *Storage {
	mock := &Storage{}

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
