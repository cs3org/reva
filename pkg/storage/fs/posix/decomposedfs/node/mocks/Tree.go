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

// Code generated by mockery v2.22.1. DO NOT EDIT.

package mocks

import (
	context "context"
	fs "io/fs"

	io "io"

	mock "github.com/stretchr/testify/mock"

	node "github.com/cs3org/reva/v2/pkg/storage/fs/posix/decomposedfs/node"
)

// Tree is an autogenerated mock type for the Tree type
type Tree struct {
	mock.Mock
}

// CreateDir provides a mock function with given fields: ctx, _a1
func (_m *Tree) CreateDir(ctx context.Context, _a1 *node.Node) error {
	ret := _m.Called(ctx, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) error); ok {
		r0 = rf(ctx, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Delete provides a mock function with given fields: ctx, _a1
func (_m *Tree) Delete(ctx context.Context, _a1 *node.Node) error {
	ret := _m.Called(ctx, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) error); ok {
		r0 = rf(ctx, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteBlob provides a mock function with given fields: _a0
func (_m *Tree) DeleteBlob(_a0 *node.Node) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(*node.Node) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetMD provides a mock function with given fields: ctx, _a1
func (_m *Tree) GetMD(ctx context.Context, _a1 *node.Node) (fs.FileInfo, error) {
	ret := _m.Called(ctx, _a1)

	var r0 fs.FileInfo
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) (fs.FileInfo, error)); ok {
		return rf(ctx, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) fs.FileInfo); ok {
		r0 = rf(ctx, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(fs.FileInfo)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *node.Node) error); ok {
		r1 = rf(ctx, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListFolder provides a mock function with given fields: ctx, _a1
func (_m *Tree) ListFolder(ctx context.Context, _a1 *node.Node) ([]*node.Node, error) {
	ret := _m.Called(ctx, _a1)

	var r0 []*node.Node
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) ([]*node.Node, error)); ok {
		return rf(ctx, _a1)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node) []*node.Node); ok {
		r0 = rf(ctx, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *node.Node) error); ok {
		r1 = rf(ctx, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Move provides a mock function with given fields: ctx, oldNode, newNode
func (_m *Tree) Move(ctx context.Context, oldNode *node.Node, newNode *node.Node) error {
	ret := _m.Called(ctx, oldNode, newNode)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, *node.Node) error); ok {
		r0 = rf(ctx, oldNode, newNode)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Propagate provides a mock function with given fields: ctx, _a1, sizeDiff
func (_m *Tree) Propagate(ctx context.Context, _a1 *node.Node, sizeDiff int64) error {
	ret := _m.Called(ctx, _a1, sizeDiff)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, int64) error); ok {
		r0 = rf(ctx, _a1, sizeDiff)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PurgeRecycleItemFunc provides a mock function with given fields: ctx, spaceid, key, purgePath
func (_m *Tree) PurgeRecycleItemFunc(ctx context.Context, spaceid string, key string, purgePath string) (*node.Node, func() error, error) {
	ret := _m.Called(ctx, spaceid, key, purgePath)

	var r0 *node.Node
	var r1 func() error
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) (*node.Node, func() error, error)); ok {
		return rf(ctx, spaceid, key, purgePath)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) *node.Node); ok {
		r0 = rf(ctx, spaceid, key, purgePath)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, string) func() error); ok {
		r1 = rf(ctx, spaceid, key, purgePath)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(func() error)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, string, string) error); ok {
		r2 = rf(ctx, spaceid, key, purgePath)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// ReadBlob provides a mock function with given fields: _a0
func (_m *Tree) ReadBlob(_a0 *node.Node) (io.ReadCloser, error) {
	ret := _m.Called(_a0)

	var r0 io.ReadCloser
	var r1 error
	if rf, ok := ret.Get(0).(func(*node.Node) (io.ReadCloser, error)); ok {
		return rf(_a0)
	}
	if rf, ok := ret.Get(0).(func(*node.Node) io.ReadCloser); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(io.ReadCloser)
		}
	}

	if rf, ok := ret.Get(1).(func(*node.Node) error); ok {
		r1 = rf(_a0)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RestoreRecycleItemFunc provides a mock function with given fields: ctx, spaceid, key, trashPath, target
func (_m *Tree) RestoreRecycleItemFunc(ctx context.Context, spaceid string, key string, trashPath string, target *node.Node) (*node.Node, *node.Node, func() error, error) {
	ret := _m.Called(ctx, spaceid, key, trashPath, target)

	var r0 *node.Node
	var r1 *node.Node
	var r2 func() error
	var r3 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, *node.Node) (*node.Node, *node.Node, func() error, error)); ok {
		return rf(ctx, spaceid, key, trashPath, target)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, *node.Node) *node.Node); ok {
		r0 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*node.Node)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, *node.Node) *node.Node); ok {
		r1 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*node.Node)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, string, string, *node.Node) func() error); ok {
		r2 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		if ret.Get(2) != nil {
			r2 = ret.Get(2).(func() error)
		}
	}

	if rf, ok := ret.Get(3).(func(context.Context, string, string, string, *node.Node) error); ok {
		r3 = rf(ctx, spaceid, key, trashPath, target)
	} else {
		r3 = ret.Error(3)
	}

	return r0, r1, r2, r3
}

// Setup provides a mock function with given fields:
func (_m *Tree) Setup() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// TouchFile provides a mock function with given fields: ctx, _a1, markprocessing, mtime
func (_m *Tree) TouchFile(ctx context.Context, _a1 *node.Node, markprocessing bool, mtime string) error {
	ret := _m.Called(ctx, _a1, markprocessing, mtime)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *node.Node, bool, string) error); ok {
		r0 = rf(ctx, _a1, markprocessing, mtime)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// WriteBlob provides a mock function with given fields: _a0, source
func (_m *Tree) WriteBlob(_a0 *node.Node, source string) error {
	ret := _m.Called(_a0, source)

	var r0 error
	if rf, ok := ret.Get(0).(func(*node.Node, string) error); ok {
		r0 = rf(_a0, source)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewTree interface {
	mock.TestingT
	Cleanup(func())
}

// NewTree creates a new instance of Tree. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewTree(t mockConstructorTestingTNewTree) *Tree {
	mock := &Tree{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
